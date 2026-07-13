package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// patPrefix identifies a raw token as a personal access token (not secret
// itself — lets callers route to introspection without attempting JWT parsing).
const patPrefix = "tbpat_"

// patPrefixDisplayLen is how many characters of the raw token are stored and
// shown in the UI to help a user recognize a token in a list (e.g. "tbpat_ab12cd").
const patPrefixDisplayLen = len(patPrefix) + 6

// PersonalAccessToken is a long-lived, revocable, opaque credential a user can
// issue for third-party clients (e.g. an MCP server). Stored as a SHA-256 hash,
// never in plaintext — mirrors RefreshToken, not the Plugin service's webhook
// secret (which must be reproduced server-side and is therefore stored plain).
type PersonalAccessToken struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Name        string
	TokenHash   string
	TokenPrefix string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	LastUsedAt  *time.Time
}

// IsValid returns true when the token is not revoked and not expired.
func (t *PersonalAccessToken) IsValid() bool {
	return t.RevokedAt == nil && time.Now().Before(t.ExpiresAt)
}

// GeneratePAT returns a new raw token (prefixed, shown to the user exactly
// once), its SHA-256 hash (persisted), and a short prefix (persisted, used to
// let the user recognize the token in a list without ever storing it in full).
func GeneratePAT() (token, hash, prefix string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generate random bytes: %w", err)
	}
	token = patPrefix + hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(token))
	hash = hex.EncodeToString(sum[:])
	prefix = token[:patPrefixDisplayLen]
	return token, hash, prefix, nil
}
