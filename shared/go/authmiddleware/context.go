// Package authmiddleware validates Bearer JWTs and injects user info into context.
package authmiddleware

import (
	"context"

	"github.com/google/uuid"
)

type contextKey int

const (
	contextKeyUserID contextKey = iota
	contextKeyEmail
)

// UserIDFromContext extracts the authenticated user's UUID from context.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(contextKeyUserID).(uuid.UUID)
	return v, ok
}

// MustUserID returns the user UUID or panics. Use only inside auth-required handlers.
func MustUserID(ctx context.Context) uuid.UUID {
	v, ok := UserIDFromContext(ctx)
	if !ok {
		panic("authmiddleware: user ID not in context — ensure JWT middleware is applied")
	}
	return v
}

// EmailFromContext extracts the authenticated user's email from context.
func EmailFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(contextKeyEmail).(string)
	return v, ok
}
