package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type ctxKey string

const ctxUserID ctxKey = "user_id"

// jwtMiddleware extracts the user UUID from the Bearer JWT subject.
// Full signature verification is handled by the gateway; here we do a
// structural parse to extract the subject claim.
func jwtMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearer(r)
		if token == "" {
			writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing authorization header")
			return
		}

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

		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
