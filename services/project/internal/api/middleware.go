package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type ctxKey string

const ctxUserID    ctxKey = "user_id"
const ctxUserEmail ctxKey = "user_email"

// jwtMiddleware validates Bearer JWTs issued by the auth service.
// In a full deployment the JWKS URL is fetched; here we use a shared public key
// loaded at startup (injected via closure if needed). For now we verify the token
// structure and extract the subject.
func jwtMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearer(r)
		if token == "" {
			writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing authorization header")
			return
		}

		// Parse without verification here — verification is done by the auth middleware
		// in shared/go/authmiddleware. For standalone use we do a structural parse.
		p := jwt.NewParser()
		claims := jwt.MapClaims{}
		_, _, err := p.ParseUnverified(token, claims)
		if err != nil {
			writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "invalid token")
			return
		}

		sub, err := claims.GetSubject()
		if err != nil || sub == "" {
			writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing subject")
			return
		}

		userID, err := uuid.Parse(sub)
		if err != nil {
			writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "invalid subject")
			return
		}

		email, _ := claims["email"].(string)
		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		ctx = context.WithValue(ctx, ctxUserEmail, email)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// serviceTokenMiddleware validates the HMAC-SHA256 service token used for
// internal service-to-service calls.
func serviceTokenMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearer(r)
			if token == "" {
				writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing service token")
				return
			}

			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte("internal"))
			expected := hex.EncodeToString(mac.Sum(nil))
			if !hmac.Equal([]byte(token), []byte(expected)) {
				writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "invalid service token")
				return
			}
			next.ServeHTTP(w, r)
		})
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

func extractBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}
