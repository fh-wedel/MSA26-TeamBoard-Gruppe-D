package api

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/teamboard/shared/go/authmiddleware"
)

type contextKey string

const ctxUserID contextKey = "userID"

// newJWTMiddleware validates the Bearer JWT against the auth service's JWKS
// (RS256 signature + iss/aud/exp enforced) via the shared authmiddleware, then
// mirrors the user ID into this service's local context key so handlers keep
// reading it through mustUserID.
func newJWTMiddleware(jwks authmiddleware.JWKSSource, issuer, audience string) func(http.Handler) http.Handler {
	verify := authmiddleware.Middleware(jwks,
		authmiddleware.WithIssuer(issuer),
		authmiddleware.WithAudience(audience),
	)
	return func(next http.Handler) http.Handler {
		return verify(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ctxUserID, authmiddleware.MustUserID(r.Context()))
			next.ServeHTTP(w, r.WithContext(ctx))
		}))
	}
}

func mustUserID(r *http.Request) uuid.UUID {
	return r.Context().Value(ctxUserID).(uuid.UUID)
}
