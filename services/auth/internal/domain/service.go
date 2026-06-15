package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AuthService defines all use-cases for the auth service.
type AuthService interface {
	Register(ctx context.Context, email, password string) (*User, error)
	Login(ctx context.Context, email, password, ip, userAgent string) (*TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (*TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	RequestPasswordReset(ctx context.Context, email string) error
	ConfirmPasswordReset(ctx context.Context, token, newPassword string) error
	GetUser(ctx context.Context, userID uuid.UUID) (*User, error)
}

// Repository is the persistence interface consumed by the domain layer.
type Repository interface {
	// Users
	CreateUser(ctx context.Context, id uuid.UUID, email, passwordHash string) (*User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error
	UpdateLastLogin(ctx context.Context, id uuid.UUID) error

	// Refresh tokens
	CreateRefreshToken(ctx context.Context, id, userID uuid.UUID, tokenHash string, expiresAt time.Time, ip, userAgent *string) (*RefreshToken, error)
	GetRefreshTokenByHash(ctx context.Context, hash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id uuid.UUID, replacedBy *uuid.UUID) error
	RevokeAllUserTokens(ctx context.Context, userID uuid.UUID) error

	// Password reset
	CreatePasswordResetToken(ctx context.Context, id, userID uuid.UUID, hash string, expiresAt time.Time) (*PasswordResetToken, error)
	GetPasswordResetTokenByHash(ctx context.Context, hash string) (*PasswordResetToken, error)
	MarkPasswordResetTokenUsed(ctx context.Context, id uuid.UUID) error

	// Signing keys
	GetActiveSigningKey(ctx context.Context) (*SigningKey, error)
	ListValidatingSigningKeys(ctx context.Context) ([]*SigningKey, error)
	CreateSigningKey(ctx context.Context, id uuid.UUID, kid, algorithm, privatePEM, publicPEM string) (*SigningKey, error)
	RetireSigningKey(ctx context.Context, id uuid.UUID) error
	DeleteRetiredSigningKeys(ctx context.Context) error

	// Login attempts
	RecordLoginAttempt(ctx context.Context, id uuid.UUID, email string, success bool, ip, userAgent *string) error
	CountRecentFailedAttempts(ctx context.Context, email string) (int64, error)

	// Outbox — used by domain to insert events within transactions
	WithTransaction(ctx context.Context, fn func(ctx context.Context, tx Repository) error) error
	InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error
}

// MailSender sends transactional email.
type MailSender interface {
	SendPasswordReset(ctx context.Context, toEmail, resetURL string) error
}

// RateLimiter checks whether an action should be allowed.
type RateLimiter interface {
	// Allow returns true when the request is within limits.
	Allow(ctx context.Context, key string, maxAttempts int, window time.Duration) (bool, error)
}
