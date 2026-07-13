package authmiddleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	testKID      = "test-key-1"
	testIssuer   = "https://auth.teamboard.local"
	testAudience = "teamboard-api"
)

// stubJWKS implements JWKSSource with a single in-memory RSA public key.
type stubJWKS struct {
	kid string
	pub *rsa.PublicKey
}

func (s stubJWKS) Key(_ context.Context, kid string) (any, error) {
	if kid != s.kid {
		return nil, jwt.ErrTokenUnverifiable
	}
	return s.pub, nil
}

func newSigner(t *testing.T) (*rsa.PrivateKey, JWKSSource) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key, stubJWKS{kid: testKID, pub: &key.PublicKey}
}

func mintRS256(t *testing.T, key *rsa.PrivateKey, sub, iss, aud string, exp time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":   sub,
		"iss":   iss,
		"aud":   aud,
		"exp":   exp.Unix(),
		"iat":   time.Now().Unix(),
		"email": "alice@teamboard.local",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = testKID
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func TestVerifyToken_Valid(t *testing.T) {
	key, jwks := newSigner(t)
	sub := uuid.NewString()
	token := mintRS256(t, key, sub, testIssuer, testAudience, time.Now().Add(time.Hour))

	claims, err := VerifyToken(context.Background(), token, jwks,
		WithIssuer(testIssuer), WithAudience(testAudience))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got, _ := claims.GetSubject(); got != sub {
		t.Errorf("sub = %q, want %q", got, sub)
	}
}

func TestVerifyToken_RejectsHS256AlgConfusion(t *testing.T) {
	_, jwks := newSigner(t)
	// Attacker signs with HS256 using the (public) kid, attempting alg confusion.
	claims := jwt.MapClaims{
		"sub": uuid.NewString(), "iss": testIssuer, "aud": testAudience,
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tok.Header["kid"] = testKID
	forged, err := tok.SignedString([]byte("anything"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := VerifyToken(context.Background(), forged, jwks,
		WithIssuer(testIssuer), WithAudience(testAudience)); err == nil {
		t.Fatal("expected HS256 token to be rejected (only RSA accepted)")
	}
}

func TestVerifyToken_RejectsExpired(t *testing.T) {
	key, jwks := newSigner(t)
	token := mintRS256(t, key, uuid.NewString(), testIssuer, testAudience, time.Now().Add(-time.Minute))
	if _, err := VerifyToken(context.Background(), token, jwks,
		WithIssuer(testIssuer), WithAudience(testAudience), WithClockSkew(0)); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestVerifyToken_RejectsWrongIssuerAndAudience(t *testing.T) {
	key, jwks := newSigner(t)

	t.Run("wrong issuer", func(t *testing.T) {
		token := mintRS256(t, key, uuid.NewString(), "evil-issuer", testAudience, time.Now().Add(time.Hour))
		if _, err := VerifyToken(context.Background(), token, jwks,
			WithIssuer(testIssuer), WithAudience(testAudience)); err == nil {
			t.Fatal("expected wrong issuer to be rejected")
		}
	})

	t.Run("wrong audience", func(t *testing.T) {
		token := mintRS256(t, key, uuid.NewString(), testIssuer, "other-api", time.Now().Add(time.Hour))
		if _, err := VerifyToken(context.Background(), token, jwks,
			WithIssuer(testIssuer), WithAudience(testAudience)); err == nil {
			t.Fatal("expected wrong audience to be rejected")
		}
	})
}

func TestMiddleware_ValidTokenInjectsUserID(t *testing.T) {
	key, jwks := newSigner(t)
	sub := uuid.NewString()
	token := mintRS256(t, key, sub, testIssuer, testAudience, time.Now().Add(time.Hour))

	var gotID uuid.UUID
	var gotEmail string
	h := Middleware(jwks, WithIssuer(testIssuer), WithAudience(testAudience))(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			gotID = MustUserID(r.Context())
			gotEmail, _ = EmailFromContext(r.Context())
		}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotID.String() != sub {
		t.Errorf("user id = %s, want %s", gotID, sub)
	}
	if gotEmail != "alice@teamboard.local" {
		t.Errorf("email = %q", gotEmail)
	}
}

func TestMiddleware_RejectsMissingAndForgedTokens(t *testing.T) {
	_, jwks := newSigner(t)
	h := Middleware(jwks, WithIssuer(testIssuer), WithAudience(testAudience))(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	t.Run("missing header", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("unsigned forged token", func(t *testing.T) {
		// A structurally valid but unsigned token (alg=none style) must be rejected.
		forged := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJhdHRhY2tlciJ9."
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+forged)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})
}
