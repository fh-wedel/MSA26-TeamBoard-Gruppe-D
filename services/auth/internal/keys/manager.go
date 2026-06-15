package keys

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/teamboard/services/auth/internal/domain"
)

// Manager caches the active signing key and implements domain.KeyManager.
// RSA private keys are encrypted with AES-256-GCM before being stored in the
// database, so a DB compromise alone cannot be used to forge JWTs.
type Manager struct {
	repo     domain.Repository
	encKey   []byte // 32-byte AES-256 key
	mu       sync.RWMutex
	cached   *domain.SigningKey
	parsed   *rsa.PrivateKey
	cacheExp time.Time
}

// NewManager creates a Manager backed by the given repository.
// encKey must be exactly 32 bytes (AES-256).
func NewManager(repo domain.Repository, encKey []byte) *Manager {
	return &Manager{repo: repo, encKey: encKey}
}

var _ domain.KeyManager = (*Manager)(nil)

// EnsureActiveKey generates and stores a new RSA key pair if no active key exists.
// The private key PEM is encrypted with AES-256-GCM before storage.
func (m *Manager) EnsureActiveKey(ctx context.Context) error {
	_, err := m.repo.GetActiveSigningKey(ctx)
	if err == nil {
		return nil
	}
	if err != domain.ErrSigningKeyNotFound {
		return fmt.Errorf("check active key: %w", err)
	}

	privatePEM, publicPEM, kid, err := domain.GenerateRSAKeyPair()
	if err != nil {
		return fmt.Errorf("generate key pair: %w", err)
	}

	encPEM, err := m.encryptPEM([]byte(privatePEM))
	if err != nil {
		return fmt.Errorf("encrypt private key: %w", err)
	}

	if _, err := m.repo.CreateSigningKey(ctx, uuid.New(), kid, "RS256", encPEM, publicPEM); err != nil {
		return fmt.Errorf("store signing key: %w", err)
	}

	slog.InfoContext(ctx, "created initial signing key", "kid", kid)
	return nil
}

// ActiveKey returns the cached active signing key, refreshing if stale (5 min TTL).
func (m *Manager) ActiveKey(ctx context.Context) (*domain.SigningKey, *rsa.PrivateKey, error) {
	m.mu.RLock()
	if m.cached != nil && time.Now().Before(m.cacheExp) {
		sk, pk := m.cached, m.parsed
		m.mu.RUnlock()
		return sk, pk, nil
	}
	m.mu.RUnlock()

	return m.refresh(ctx)
}

func (m *Manager) refresh(ctx context.Context) (*domain.SigningKey, *rsa.PrivateKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// double-checked locking
	if m.cached != nil && time.Now().Before(m.cacheExp) {
		return m.cached, m.parsed, nil
	}

	sk, err := m.repo.GetActiveSigningKey(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("get active signing key: %w", err)
	}

	plainPEM, err := m.decryptPEM(sk.PrivateKeyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt private key (kid=%s): %w", sk.KID, err)
	}

	pk, err := domain.ParseRSAPrivateKey(plainPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("parse private key (kid=%s): %w", sk.KID, err)
	}

	m.cached = sk
	m.parsed = pk
	m.cacheExp = time.Now().Add(5 * time.Minute)
	return sk, pk, nil
}

// encryptPEM encrypts plaintext with AES-256-GCM and returns a base64-encoded
// string containing [nonce || ciphertext].
func (m *Manager) encryptPEM(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(m.encKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptPEM reverses encryptPEM.
func (m *Manager) decryptPEM(encoded string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	block, err := aes.NewCipher(m.encKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], nil)
}
