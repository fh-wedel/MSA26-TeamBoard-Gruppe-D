package api

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/teamboard/shared/go/authmiddleware"
)

type contextKey string

const ctxUserID contextKey = "userID"

// tokenVerifier holds everything needed to validate a user JWT (RS256 against
// the auth service's JWKS, with iss/aud/exp enforced). It is shared by the HTTP
// middleware and the WebSocket upgrade path (which carries the token in a query
// parameter instead of a header).
type tokenVerifier struct {
	jwks     authmiddleware.JWKSSource
	issuer   string
	audience string
}

func newTokenVerifier(jwks authmiddleware.JWKSSource, issuer, audience string) *tokenVerifier {
	return &tokenVerifier{jwks: jwks, issuer: issuer, audience: audience}
}

// verify validates a raw token string and returns the authenticated user ID.
func (v *tokenVerifier) verify(ctx context.Context, token string) (uuid.UUID, error) {
	claims, err := authmiddleware.VerifyToken(ctx, token, v.jwks,
		authmiddleware.WithIssuer(v.issuer),
		authmiddleware.WithAudience(v.audience),
	)
	if err != nil {
		return uuid.Nil, err
	}
	sub, err := claims.GetSubject()
	if err != nil || sub == "" {
		return uuid.Nil, ErrBadToken
	}
	id, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, ErrBadToken
	}
	return id, nil
}

// middleware validates the Authorization: Bearer header and injects the user ID.
func (v *tokenVerifier) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		auth := r.Header.Get("Authorization")
		if len(auth) <= len(prefix) {
			http.Error(w, `{"error":"missing token"}`, http.StatusUnauthorized)
			return
		}
		userID, err := v.verify(r.Context(), auth[len(prefix):])
		if err != nil {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func mustUserID(r *http.Request) uuid.UUID {
	return r.Context().Value(ctxUserID).(uuid.UUID)
}

type errBadToken struct{}

func (e errBadToken) Error() string { return "bad token" }

var ErrBadToken = errBadToken{}
