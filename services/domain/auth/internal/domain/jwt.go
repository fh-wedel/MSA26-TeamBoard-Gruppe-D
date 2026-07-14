package domain

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
)

// JWTClaims holds the payload for TeamBoard access tokens.
type JWTClaims struct {
	Email string `json:"email"`
	Scope string `json:"scope"`
	jwt.RegisteredClaims
}

// IssueAccessToken signs a new RS256 JWT for the given user.
func IssueAccessToken(user *User, kid string, privateKey *rsa.PrivateKey, issuer, audience string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		Email: user.Email,
		Scope: "user",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   user.ID.String(),
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        ulid.Make().String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid

	signed, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signed, nil
}

// GenerateRefreshToken returns a cryptographically random opaque token and its SHA-256 hex hash.
func GenerateRefreshToken() (token, hash string, err error) {
	return generateOpaqueToken(32)
}

// GeneratePasswordResetToken returns a cryptographically random reset token and its SHA-256 hex hash.
func GeneratePasswordResetToken() (token, hash string, err error) {
	return generateOpaqueToken(32)
}

func generateOpaqueToken(nBytes int) (token, hash string, err error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate random bytes: %w", err)
	}
	token = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(token))
	hash = hex.EncodeToString(sum[:])
	return token, hash, nil
}

// ParseRSAPrivateKey parses a PEM-encoded PKCS#1 or PKCS#8 RSA private key.
func ParseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	return jwt.ParseRSAPrivateKeyFromPEM(pemBytes)
}

// ParseRSAPublicKey parses a PEM-encoded RSA public key.
func ParseRSAPublicKey(pemBytes []byte) (*rsa.PublicKey, error) {
	return jwt.ParseRSAPublicKeyFromPEM(pemBytes)
}

// GenerateRSAKeyPair generates a new 2048-bit RSA key pair and returns PEM-encoded strings and a KID.
func GenerateRSAKeyPair() (privatePEM, publicPEM, kid string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", "", fmt.Errorf("generate RSA key: %w", err)
	}

	privBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	privatePEM = string(pem.EncodeToMemory(privBlock))

	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", "", fmt.Errorf("marshal public key: %w", err)
	}
	pubBlock := &pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}
	publicPEM = string(pem.EncodeToMemory(pubBlock))

	// KID is the first 16 hex chars of the public key DER hash
	sum := sha256.Sum256(pubDER)
	kid = "key-" + hex.EncodeToString(sum[:])[:16]

	return privatePEM, publicPEM, kid, nil
}

// UserIDFromTokenUnverified parses the sub claim without signature verification.
// Use only for logging — never for auth decisions.
func UserIDFromTokenUnverified(tokenStr string) (uuid.UUID, error) {
	p := jwt.NewParser()
	token, _, err := p.ParseUnverified(tokenStr, &JWTClaims{})
	if err != nil {
		return uuid.Nil, err
	}
	sub, err := token.Claims.GetSubject()
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(sub)
}

