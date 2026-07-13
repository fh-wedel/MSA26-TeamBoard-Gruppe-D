package servicetoken

import (
	"context"
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt/v5"

	"github.com/teamboard/shared/go/httputil"
)

// Claims holds verified service token claims.
type Claims struct {
	Service  string
	Audience string
}

// Verifier validates incoming service tokens.
type Verifier interface {
	Verify(ctx context.Context, token string) (*Claims, error)
}

type verifier struct {
	secret   []byte
	audience string
}

// NewVerifier creates a Verifier that accepts tokens signed with secret for audience.
func NewVerifier(secret, audience string) Verifier {
	return &verifier{secret: []byte(secret), audience: audience}
}

func (v *verifier) Verify(_ context.Context, tokenStr string) (*Claims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return v.secret, nil
	},
		jwt.WithAudience(v.audience),
		jwt.WithIssuer("teamboard-internal"),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid service token: %w", err)
	}

	reg, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}

	sub, _ := reg.GetSubject()
	aud, _ := reg.GetAudience()
	audStr := ""
	if len(aud) > 0 {
		audStr = aud[0]
	}
	return &Claims{Service: sub, Audience: audStr}, nil
}

// RequireServiceToken returns middleware that rejects requests without a valid service token.
func RequireServiceToken(v Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const prefix = "Bearer "
			h := r.Header.Get("Authorization")
			if len(h) <= len(prefix) {
				httputil.WriteProblem(w, r, http.StatusUnauthorized, "missing service token")
				return
			}
			tokenStr := h[len(prefix):]
			if _, err := v.Verify(r.Context(), tokenStr); err != nil {
				httputil.WriteProblem(w, r, http.StatusUnauthorized, "invalid service token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
