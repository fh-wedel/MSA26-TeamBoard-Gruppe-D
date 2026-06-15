package api

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/teamboard/services/auth/internal/domain"
)

type contextKey string

const ctxUserID contextKey = "user_id"

// requireAuth validates a Bearer RS256 JWT and injects the user UUID into context.
// It resolves the signing public key by matching the JWT's kid against the DB.
func requireAuth(repo domain.Repository, issuer, audience string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractBearerToken(r)
			if tokenStr == "" {
				writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing or invalid Authorization header")
				return
			}

			claims := &domain.JWTClaims{}
			_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				kid, _ := t.Header["kid"].(string)

				keys, err := repo.ListValidatingSigningKeys(r.Context())
				if err != nil {
					return nil, fmt.Errorf("list signing keys: %w", err)
				}
				for _, k := range keys {
					if k.KID == kid {
						return domain.ParseRSAPublicKey([]byte(k.PublicKeyPEM))
					}
				}
				return nil, fmt.Errorf("no key with kid %q", kid)
			},
				jwt.WithIssuer(issuer),
				jwt.WithValidMethods([]string{"RS256"}),
			)
			if err != nil {
				writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
				return
			}

			// Validate audience manually (jwt/v5 has no WithAudiences parser option)
			auds, _ := claims.GetAudience()
			if !slices.Contains(auds, audience) {
				writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "invalid token audience")
				return
			}

			sub, err := claims.GetSubject()
			if err != nil {
				writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing subject claim")
				return
			}
			userID, err := uuid.Parse(sub)
			if err != nil {
				writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "invalid subject claim")
				return
			}

			ctx := context.WithValue(r.Context(), ctxUserID, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func userIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ctxUserID).(uuid.UUID)
	return id, ok
}

func extractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}
