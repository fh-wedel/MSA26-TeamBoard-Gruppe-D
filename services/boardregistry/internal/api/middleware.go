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

const ctxUserID ctxKey = "user_id"

// jwtMiddleware performs a structural parse of the Bearer JWT issued by the auth
// service and extracts the subject (user id). Signature verification happens at
// the gateway; this mirrors the project service's middleware.
func jwtMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearer(r)
		if token == "" {
			writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing authorization header")
			return
		}
		claims := jwt.MapClaims{}
		if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
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
		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// serviceTokenMiddleware validates the HMAC-SHA256("internal") service token used
// for internal service-to-service calls (same scheme as the project service).
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

func extractBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}
