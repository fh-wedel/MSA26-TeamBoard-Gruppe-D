package api

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/teamboard/shared/go/authmiddleware"
)

type ctxKey string

const ctxUserID ctxKey = "user_id"
const ctxUserEmail ctxKey = "user_email"

// newJWTMiddleware validates the Bearer JWT against the auth service's JWKS
// (RS256 signature + iss/aud/exp enforced) via the shared authmiddleware, then
// mirrors the authenticated identity into this service's local context keys so
// existing handlers keep reading it through mustUserID/emailFromContext.
func newJWTMiddleware(jwks authmiddleware.JWKSSource, issuer, audience string) func(http.Handler) http.Handler {
	verify := authmiddleware.Middleware(jwks,
		authmiddleware.WithIssuer(issuer),
		authmiddleware.WithAudience(audience),
	)
	return func(next http.Handler) http.Handler {
		// verify writes 401 and stops the chain on any invalid token; the inner
		// handler only runs once the user ID is guaranteed to be in context.
		return verify(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ctxUserID, authmiddleware.MustUserID(r.Context()))
			if email, ok := authmiddleware.EmailFromContext(r.Context()); ok {
				ctx = context.WithValue(ctx, ctxUserEmail, email)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		}))
	}
}

func mustUserID(r *http.Request) uuid.UUID {
	id, _ := r.Context().Value(ctxUserID).(uuid.UUID)
	return id
}

func userIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ctxUserID).(uuid.UUID)
	return id, ok
}

func emailFromContext(ctx context.Context) (string, bool) {
	email, ok := ctx.Value(ctxUserEmail).(string)
	return email, ok
}
