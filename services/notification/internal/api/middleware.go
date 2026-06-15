package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type contextKey string

const ctxUserID contextKey = "userID"

func jwtMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, `{"error":"missing token"}`, http.StatusUnauthorized)
			return
		}
		userID, err := extractSubFromToken(strings.TrimPrefix(auth, "Bearer "))
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

func extractSubFromToken(token string) (uuid.UUID, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return uuid.Nil, ErrBadToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return uuid.Nil, ErrBadToken
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Sub == "" {
		return uuid.Nil, ErrBadToken
	}
	id, err := uuid.Parse(claims.Sub)
	if err != nil {
		return uuid.Nil, ErrBadToken
	}
	return id, nil
}

type errBadToken struct{}

func (e errBadToken) Error() string { return "bad token" }

var ErrBadToken = errBadToken{}
