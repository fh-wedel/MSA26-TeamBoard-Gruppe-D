package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/teamboard/services/auth/internal/domain"
)

func TestGenerateRefreshToken_Uniqueness(t *testing.T) {
	t1, h1, err := domain.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	t2, h2, _ := domain.GenerateRefreshToken()

	if t1 == t2 {
		t.Error("tokens should be unique")
	}
	if h1 == h2 {
		t.Error("hashes should be unique")
	}
	if len(t1) != 64 { // 32 bytes hex-encoded
		t.Errorf("expected 64-char token, got %d", len(t1))
	}
	if len(h1) != 64 { // SHA-256 hex-encoded
		t.Errorf("expected 64-char hash, got %d", len(h1))
	}
}

func TestGeneratePasswordResetToken(t *testing.T) {
	tok, hash, err := domain.GeneratePasswordResetToken()
	if err != nil {
		t.Fatalf("GeneratePasswordResetToken: %v", err)
	}
	if tok == "" || hash == "" {
		t.Error("expected non-empty token and hash")
	}
}

func TestGenerateRSAKeyPair(t *testing.T) {
	priv, pub, kid, err := domain.GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair: %v", err)
	}
	if priv == "" || pub == "" || kid == "" {
		t.Error("expected non-empty PEM strings and kid")
	}
	if _, err := domain.ParseRSAPrivateKey([]byte(priv)); err != nil {
		t.Errorf("ParseRSAPrivateKey: %v", err)
	}
	if _, err := domain.ParseRSAPublicKey([]byte(pub)); err != nil {
		t.Errorf("ParseRSAPublicKey: %v", err)
	}
}

func TestIssueAccessToken_ValidClaims(t *testing.T) {
	priv, _, kid, err := domain.GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privateKey, err := domain.ParseRSAPrivateKey([]byte(priv))
	if err != nil {
		t.Fatalf("parse private key: %v", err)
	}

	user := &domain.User{
		ID:        uuid.New(),
		Email:     "test@example.com",
		CreatedAt: time.Now(),
	}

	token, err := domain.IssueAccessToken(user, kid, privateKey, "issuer", "audience", 15*time.Minute)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty JWT")
	}

	// Parse sub without verification (for logging use only)
	gotID, err := domain.UserIDFromTokenUnverified(token)
	if err != nil {
		t.Fatalf("UserIDFromTokenUnverified: %v", err)
	}
	if gotID != user.ID {
		t.Errorf("expected user ID %s, got %s", user.ID, gotID)
	}
}
