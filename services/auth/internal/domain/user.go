package domain

import (
	"time"

	"github.com/google/uuid"
)

// User is the core identity entity managed by the auth service.
type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
	LastLoginAt  *time.Time
}
