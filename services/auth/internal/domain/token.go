package domain

import (
	"time"

	"github.com/google/uuid"
)

// RefreshToken represents a single-use opaque token stored as a SHA-256 hash.
type RefreshToken struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  string
	IssuedAt   time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	ReplacedBy *uuid.UUID
}

// IsValid returns true when the token is not expired and not revoked.
func (t *RefreshToken) IsValid() bool {
	return t.RevokedAt == nil && time.Now().Before(t.ExpiresAt)
}

// PasswordResetToken is a one-time token for the password-reset flow.
type PasswordResetToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	IssuedAt  time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// IsValid returns true when the token is unused and not expired.
func (t *PasswordResetToken) IsValid() bool {
	return t.UsedAt == nil && time.Now().Before(t.ExpiresAt)
}

// TokenPair is returned on successful login or refresh.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int    // seconds until access token expires
	TokenType    string // always "Bearer"
}
