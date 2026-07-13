package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/teamboard/services/auth/internal/domain"
)

func TestError_ErrorString(t *testing.T) {
	e := &domain.Error{Code: "test_code", Message: "test message"}
	if got := e.Error(); got != "test_code: test message" {
		t.Errorf("Error() = %q, want %q", got, "test_code: test message")
	}
}

func TestError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	e := &domain.Error{Code: "wrapped", Message: "outer", Cause: cause}
	if e.Unwrap() != cause {
		t.Error("Unwrap should return the cause error")
	}
	if e.GetCode() != "wrapped" {
		t.Errorf("GetCode() = %q, want %q", e.GetCode(), "wrapped")
	}
	// errors.Is traverses Unwrap chain
	if !errors.Is(e, cause) {
		t.Error("errors.Is should find cause via Unwrap")
	}
}

func TestSentinelErrors_Codes(t *testing.T) {
	cases := []struct {
		err  *domain.Error
		code string
	}{
		{domain.ErrEmailTaken, "email_taken"},
		{domain.ErrInvalidCredentials, "invalid_credentials"},
		{domain.ErrPasswordTooWeak, "password_too_weak"},
		{domain.ErrTokenInvalid, "token_invalid"},
		{domain.ErrTokenRevoked, "token_revoked"},
		{domain.ErrUserNotFound, "user_not_found"},
		{domain.ErrRateLimited, "rate_limited"},
		{domain.ErrSigningKeyNotFound, "signing_key_not_found"},
	}
	for _, tc := range cases {
		if tc.err.GetCode() != tc.code {
			t.Errorf("%v: GetCode() = %q, want %q", tc.err, tc.err.GetCode(), tc.code)
		}
		if tc.err.Error() == "" {
			t.Errorf("%v: Error() must not be empty", tc.code)
		}
	}
}

func TestSigningKey_IsActive(t *testing.T) {
	now := time.Now()

	t.Run("active", func(t *testing.T) {
		sk := &domain.SigningKey{}
		if !sk.IsActive() {
			t.Error("expected IsActive() = true when both times are nil")
		}
	})
	t.Run("retired", func(t *testing.T) {
		sk := &domain.SigningKey{RetiredAt: &now}
		if sk.IsActive() {
			t.Error("expected IsActive() = false when RetiredAt is set")
		}
	})
	t.Run("deleted", func(t *testing.T) {
		sk := &domain.SigningKey{DeletedAt: &now}
		if sk.IsActive() {
			t.Error("expected IsActive() = false when DeletedAt is set")
		}
	})
}
