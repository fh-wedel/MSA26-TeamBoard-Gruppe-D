package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/teamboard/services/auth/internal/domain"
)

func TestRefreshToken_IsValid(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		tok := &domain.RefreshToken{
			ID:        uuid.New(),
			ExpiresAt: time.Now().Add(time.Hour),
		}
		if !tok.IsValid() {
			t.Error("expected IsValid() = true")
		}
	})

	t.Run("expired", func(t *testing.T) {
		tok := &domain.RefreshToken{
			ID:        uuid.New(),
			ExpiresAt: time.Now().Add(-time.Second),
		}
		if tok.IsValid() {
			t.Error("expected IsValid() = false for expired token")
		}
	})

	t.Run("revoked", func(t *testing.T) {
		now := time.Now()
		tok := &domain.RefreshToken{
			ID:        uuid.New(),
			ExpiresAt: time.Now().Add(time.Hour),
			RevokedAt: &now,
		}
		if tok.IsValid() {
			t.Error("expected IsValid() = false for revoked token")
		}
	})
}

func TestPasswordResetToken_IsValid(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		tok := &domain.PasswordResetToken{
			ID:        uuid.New(),
			ExpiresAt: time.Now().Add(time.Hour),
		}
		if !tok.IsValid() {
			t.Error("expected IsValid() = true")
		}
	})

	t.Run("expired", func(t *testing.T) {
		tok := &domain.PasswordResetToken{
			ID:        uuid.New(),
			ExpiresAt: time.Now().Add(-time.Second),
		}
		if tok.IsValid() {
			t.Error("expected IsValid() = false for expired token")
		}
	})

	t.Run("used", func(t *testing.T) {
		now := time.Now()
		tok := &domain.PasswordResetToken{
			ID:        uuid.New(),
			ExpiresAt: time.Now().Add(time.Hour),
			UsedAt:    &now,
		}
		if tok.IsValid() {
			t.Error("expected IsValid() = false for used token")
		}
	})
}
