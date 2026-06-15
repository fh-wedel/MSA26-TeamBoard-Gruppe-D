package domain

import (
	"time"

	"github.com/google/uuid"
)

// SigningKey holds an RSA key pair used to sign JWTs.
type SigningKey struct {
	ID            uuid.UUID
	KID           string
	Algorithm     string
	PrivateKeyPEM string
	PublicKeyPEM  string
	CreatedAt     time.Time
	ActivatedAt   time.Time
	RetiredAt     *time.Time
	DeletedAt     *time.Time
}

// IsActive returns true when the key is the current signing key.
func (k *SigningKey) IsActive() bool {
	return k.RetiredAt == nil && k.DeletedAt == nil
}
