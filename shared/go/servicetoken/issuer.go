// Package servicetoken provides short-lived JWTs for service-to-service authentication.
package servicetoken

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oklog/ulid/v2"
)

const tokenLifetime = 60 * time.Second

// Issuer creates short-lived service tokens.
type Issuer interface {
	Issue(ctx context.Context, callerService, audience string) (string, error)
}

type issuer struct {
	secret []byte
}

// NewIssuer creates an Issuer using HMAC-SHA256 with the given secret.
func NewIssuer(secret string) Issuer {
	return &issuer{secret: []byte(secret)}
}

func (iss *issuer) Issue(_ context.Context, callerService, audience string) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    "teamboard-internal",
		Subject:   callerService,
		Audience:  jwt.ClaimStrings{audience},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(tokenLifetime)),
		ID:        ulid.Make().String(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(iss.secret)
	if err != nil {
		return "", fmt.Errorf("sign service token: %w", err)
	}
	return signed, nil
}
