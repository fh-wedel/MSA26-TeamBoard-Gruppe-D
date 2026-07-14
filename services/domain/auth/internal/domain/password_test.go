package domain_test

import (
	"strings"
	"testing"

	"github.com/teamboard/services/domain/auth/internal/domain"
)

func TestHashPassword_RoundTrip(t *testing.T) {
	hash, err := domain.HashPassword("CorrectHorseBattery1!")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("expected argon2id PHC string, got %q", hash[:20])
	}
	if !domain.VerifyPassword("CorrectHorseBattery1!", hash) {
		t.Error("VerifyPassword: expected true for correct password")
	}
	if domain.VerifyPassword("wrongpassword", hash) {
		t.Error("VerifyPassword: expected false for wrong password")
	}
}

func TestHashPassword_UniqueHashes(t *testing.T) {
	h1, _ := domain.HashPassword("samepassword")
	h2, _ := domain.HashPassword("samepassword")
	if h1 == h2 {
		t.Error("expected distinct hashes due to unique salts")
	}
}

func TestVerifyPassword_MalformedHash(t *testing.T) {
	cases := []string{
		"",
		"$argon2id$v=19$m=65536,t=3,p=2$invalidsalt",
		"notahash",
		"$argon2id$v=19$m=65536,t=3,p=2$",
	}
	for _, c := range cases {
		if domain.VerifyPassword("password", c) {
			t.Errorf("expected false for malformed hash %q", c)
		}
	}
}

func TestCheckPasswordPolicy(t *testing.T) {
	t.Run("too_short", func(t *testing.T) {
		if err := domain.CheckPasswordPolicy("abc", 8, 128); err != domain.ErrPasswordTooWeak {
			t.Errorf("expected ErrPasswordTooWeak, got %v", err)
		}
	})
	t.Run("too_long", func(t *testing.T) {
		if err := domain.CheckPasswordPolicy(strings.Repeat("a", 129), 8, 128); err != domain.ErrPasswordTooWeak {
			t.Errorf("expected ErrPasswordTooWeak, got %v", err)
		}
	})
	t.Run("valid", func(t *testing.T) {
		if err := domain.CheckPasswordPolicy("Valid123!", 8, 128); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("exact_min", func(t *testing.T) {
		if err := domain.CheckPasswordPolicy("12345678", 8, 128); err != nil {
			t.Errorf("unexpected error at min length: %v", err)
		}
	})
}
