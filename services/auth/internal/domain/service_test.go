package domain_test

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/teamboard/services/auth/internal/domain"
)

// ── Fakes ─────────────────────────────────────────────────────────────────────

type fakeRepo struct {
	users         map[string]*domain.User
	usersByID     map[uuid.UUID]*domain.User
	refreshTokens map[string]*domain.RefreshToken
	resetTokens   map[string]*domain.PasswordResetToken
	pats          map[string]*domain.PersonalAccessToken
	activeKey     *domain.SigningKey
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		users:         make(map[string]*domain.User),
		usersByID:     make(map[uuid.UUID]*domain.User),
		refreshTokens: make(map[string]*domain.RefreshToken),
		resetTokens:   make(map[string]*domain.PasswordResetToken),
		pats:          make(map[string]*domain.PersonalAccessToken),
	}
}

func (r *fakeRepo) CreateUser(_ context.Context, id uuid.UUID, email, hash string) (*domain.User, error) {
	if _, exists := r.users[email]; exists {
		return nil, domain.ErrEmailTaken
	}
	u := &domain.User{ID: id, Email: email, PasswordHash: hash, CreatedAt: time.Now()}
	r.users[email] = u
	r.usersByID[id] = u
	return u, nil
}

func (r *fakeRepo) GetUserByEmail(_ context.Context, email string) (*domain.User, error) {
	u, ok := r.users[email]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}

func (r *fakeRepo) GetUserByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	u, ok := r.usersByID[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}

func (r *fakeRepo) UpdatePasswordHash(_ context.Context, id uuid.UUID, hash string) error {
	u, ok := r.usersByID[id]
	if !ok {
		return domain.ErrUserNotFound
	}
	u.PasswordHash = hash
	r.users[u.Email] = u
	return nil
}

func (r *fakeRepo) UpdateLastLogin(_ context.Context, _ uuid.UUID) error { return nil }

func (r *fakeRepo) CreateRefreshToken(_ context.Context, id, userID uuid.UUID, hash string, exp time.Time, ip, ua *string) (*domain.RefreshToken, error) {
	tok := &domain.RefreshToken{ID: id, UserID: userID, TokenHash: hash, ExpiresAt: exp, IssuedAt: time.Now()}
	r.refreshTokens[hash] = tok
	return tok, nil
}

func (r *fakeRepo) GetRefreshTokenByHash(_ context.Context, hash string) (*domain.RefreshToken, error) {
	tok, ok := r.refreshTokens[hash]
	if !ok {
		return nil, domain.ErrTokenInvalid
	}
	return tok, nil
}

func (r *fakeRepo) RevokeRefreshToken(_ context.Context, id uuid.UUID, replacedBy *uuid.UUID) error {
	now := time.Now()
	for _, tok := range r.refreshTokens {
		if tok.ID == id {
			tok.RevokedAt = &now
			tok.ReplacedBy = replacedBy
		}
	}
	return nil
}

func (r *fakeRepo) RevokeAllUserTokens(_ context.Context, userID uuid.UUID) error {
	now := time.Now()
	for _, tok := range r.refreshTokens {
		if tok.UserID == userID {
			tok.RevokedAt = &now
		}
	}
	return nil
}

func (r *fakeRepo) CreatePasswordResetToken(_ context.Context, id, userID uuid.UUID, hash string, exp time.Time) (*domain.PasswordResetToken, error) {
	tok := &domain.PasswordResetToken{ID: id, UserID: userID, TokenHash: hash, ExpiresAt: exp, IssuedAt: time.Now()}
	r.resetTokens[hash] = tok
	return tok, nil
}

func (r *fakeRepo) GetPasswordResetTokenByHash(_ context.Context, hash string) (*domain.PasswordResetToken, error) {
	tok, ok := r.resetTokens[hash]
	if !ok {
		return nil, domain.ErrTokenInvalid
	}
	return tok, nil
}

func (r *fakeRepo) MarkPasswordResetTokenUsed(_ context.Context, id uuid.UUID) error {
	now := time.Now()
	for _, tok := range r.resetTokens {
		if tok.ID == id {
			tok.UsedAt = &now
		}
	}
	return nil
}

func (r *fakeRepo) GetActiveSigningKey(_ context.Context) (*domain.SigningKey, error) {
	if r.activeKey == nil {
		return nil, domain.ErrSigningKeyNotFound
	}
	return r.activeKey, nil
}

func (r *fakeRepo) ListValidatingSigningKeys(_ context.Context) ([]*domain.SigningKey, error) {
	if r.activeKey == nil {
		return nil, nil
	}
	return []*domain.SigningKey{r.activeKey}, nil
}

func (r *fakeRepo) CreateSigningKey(_ context.Context, id uuid.UUID, kid, alg, priv, pub string) (*domain.SigningKey, error) {
	sk := &domain.SigningKey{ID: id, KID: kid, Algorithm: alg, PrivateKeyPEM: priv, PublicKeyPEM: pub, CreatedAt: time.Now(), ActivatedAt: time.Now()}
	r.activeKey = sk
	return sk, nil
}

func (r *fakeRepo) RetireSigningKey(_ context.Context, _ uuid.UUID) error       { return nil }
func (r *fakeRepo) DeleteRetiredSigningKeys(_ context.Context) error             { return nil }
func (r *fakeRepo) RecordLoginAttempt(_ context.Context, _ uuid.UUID, _ string, _ bool, _, _ *string) error {
	return nil
}
func (r *fakeRepo) CountRecentFailedAttempts(_ context.Context, _ string) (int64, error) {
	return 0, nil
}
func (r *fakeRepo) InsertOutboxEvent(_ context.Context, _, _ uuid.UUID, _ string, _ []byte) error {
	return nil
}
func (r *fakeRepo) WithTransaction(ctx context.Context, fn func(context.Context, domain.Repository) error) error {
	return fn(ctx, r)
}

func (r *fakeRepo) CreatePersonalAccessToken(_ context.Context, id, userID uuid.UUID, name, tokenHash, tokenPrefix string, expiresAt time.Time) (*domain.PersonalAccessToken, error) {
	pat := &domain.PersonalAccessToken{
		ID: id, UserID: userID, Name: name, TokenHash: tokenHash, TokenPrefix: tokenPrefix,
		CreatedAt: time.Now(), ExpiresAt: expiresAt,
	}
	r.pats[tokenHash] = pat
	return pat, nil
}

func (r *fakeRepo) GetPersonalAccessTokenByHash(_ context.Context, hash string) (*domain.PersonalAccessToken, error) {
	pat, ok := r.pats[hash]
	if !ok {
		return nil, domain.ErrTokenInvalid
	}
	return pat, nil
}

func (r *fakeRepo) ListPersonalAccessTokensByUser(_ context.Context, userID uuid.UUID) ([]*domain.PersonalAccessToken, error) {
	var pats []*domain.PersonalAccessToken
	for _, pat := range r.pats {
		if pat.UserID == userID {
			pats = append(pats, pat)
		}
	}
	return pats, nil
}

func (r *fakeRepo) RevokePersonalAccessToken(_ context.Context, id, userID uuid.UUID) error {
	now := time.Now()
	for _, pat := range r.pats {
		if pat.ID == id && pat.UserID == userID {
			pat.RevokedAt = &now
		}
	}
	return nil
}

func (r *fakeRepo) TouchPersonalAccessTokenLastUsed(_ context.Context, id uuid.UUID) error {
	now := time.Now()
	for _, pat := range r.pats {
		if pat.ID == id {
			pat.LastUsedAt = &now
		}
	}
	return nil
}

// testKeyManager holds a pre-generated RSA key pair.
type testKeyManager struct {
	sk  *domain.SigningKey
	key *rsa.PrivateKey
}

func newTestKeyManager(t *testing.T) *testKeyManager {
	t.Helper()
	priv, pub, kid, err := domain.GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	pk, err := domain.ParseRSAPrivateKey([]byte(priv))
	if err != nil {
		t.Fatalf("parse private key: %v", err)
	}
	sk := &domain.SigningKey{
		ID: uuid.New(), KID: kid, Algorithm: "RS256",
		PrivateKeyPEM: priv, PublicKeyPEM: pub,
		CreatedAt: time.Now(), ActivatedAt: time.Now(),
	}
	return &testKeyManager{sk: sk, key: pk}
}

func (m *testKeyManager) EnsureActiveKey(_ context.Context) error { return nil }
func (m *testKeyManager) ActiveKey(_ context.Context) (*domain.SigningKey, *rsa.PrivateKey, error) {
	return m.sk, m.key, nil
}

// fakeLimiter always allows all requests.
type fakeLimiter struct{}

func (l *fakeLimiter) Allow(_ context.Context, _ string, _ int, _ time.Duration) (bool, error) {
	return true, nil
}

// capturingMail records the last reset URL it received.
type capturingMail struct{ lastURL string }

func (m *capturingMail) SendPasswordReset(_ context.Context, _, url string) error {
	m.lastURL = url
	return nil
}

// ── Builder ───────────────────────────────────────────────────────────────────

func newTestService(t *testing.T) (domain.AuthService, *fakeRepo, *capturingMail) {
	t.Helper()
	repo := newFakeRepo()
	keyMgr := newTestKeyManager(t)
	repo.activeKey = keyMgr.sk
	mail := &capturingMail{}
	svc := domain.NewAuthService(repo, mail, &fakeLimiter{}, keyMgr, domain.ServiceConfig{
		Issuer: "test", Audience: "test",
		AccessTokenTTL:   15 * time.Minute,
		RefreshTokenTTL:  720 * time.Hour,
		PasswordResetTTL: time.Hour,
		PasswordResetURL: "http://localhost/reset",
		PasswordMinLen:   8,
		PasswordMaxLen:   128,
	})
	return svc, repo, mail
}

// sha256Hex reproduces the hash logic used internally by the domain service.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ── Register ──────────────────────────────────────────────────────────────────

func TestAuthService_Register_HappyPath(t *testing.T) {
	svc, _, _ := newTestService(t)
	user, err := svc.Register(context.Background(), "alice@example.com", "Password1!")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.Email != "alice@example.com" {
		t.Errorf("email: want alice@example.com, got %s", user.Email)
	}
	if user.ID == uuid.Nil {
		t.Error("expected non-nil user ID")
	}
}

func TestAuthService_Register_DuplicateEmail(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "bob@example.com", "Password1!")
	_, err := svc.Register(ctx, "bob@example.com", "Password1!")
	if !errors.Is(err, domain.ErrEmailTaken) {
		t.Errorf("expected ErrEmailTaken, got %v", err)
	}
}

func TestAuthService_Register_WeakPassword(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Register(context.Background(), "user@example.com", "short")
	if !errors.Is(err, domain.ErrPasswordTooWeak) {
		t.Errorf("expected ErrPasswordTooWeak, got %v", err)
	}
}

// ── Login ─────────────────────────────────────────────────────────────────────

func TestAuthService_Login_HappyPath(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "alice@example.com", "Password1!")

	pair, err := svc.Login(ctx, "alice@example.com", "Password1!", "127.0.0.1", "agent/1.0")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Error("expected non-empty token pair")
	}
	if pair.TokenType != "Bearer" {
		t.Errorf("TokenType: want Bearer, got %s", pair.TokenType)
	}
	if pair.ExpiresIn <= 0 {
		t.Errorf("ExpiresIn: want >0, got %d", pair.ExpiresIn)
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "alice@example.com", "Password1!")
	_, err := svc.Login(ctx, "alice@example.com", "wrongpassword", "", "")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthService_Login_UnknownEmail(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Login(context.Background(), "nobody@example.com", "Password1!", "", "")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials for unknown email, got %v", err)
	}
}

// ── Refresh ───────────────────────────────────────────────────────────────────

func TestAuthService_Refresh_HappyPath(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "alice@example.com", "Password1!")
	pair, _ := svc.Login(ctx, "alice@example.com", "Password1!", "", "")

	newPair, err := svc.Refresh(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if newPair.RefreshToken == pair.RefreshToken {
		t.Error("expected rotated refresh token")
	}
	if newPair.AccessToken == "" {
		t.Error("expected non-empty access token after refresh")
	}
}

func TestAuthService_Refresh_InvalidToken(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Refresh(context.Background(), "notavalidtoken")
	if !errors.Is(err, domain.ErrTokenInvalid) {
		t.Errorf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestAuthService_Refresh_ReuseDetected(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "alice@example.com", "Password1!")
	pair, _ := svc.Login(ctx, "alice@example.com", "Password1!", "", "")

	// First use is valid
	_, _ = svc.Refresh(ctx, pair.RefreshToken)

	// Second use of the same token must be detected as reuse
	_, err := svc.Refresh(ctx, pair.RefreshToken)
	if !errors.Is(err, domain.ErrTokenRevoked) {
		t.Errorf("expected ErrTokenRevoked on reuse, got %v", err)
	}
}

// ── Logout ────────────────────────────────────────────────────────────────────

func TestAuthService_Logout(t *testing.T) {
	svc, repo, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "alice@example.com", "Password1!")
	pair, _ := svc.Login(ctx, "alice@example.com", "Password1!", "", "")

	if err := svc.Logout(ctx, pair.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	hash := sha256Hex(pair.RefreshToken)
	tok := repo.refreshTokens[hash]
	if tok == nil || tok.RevokedAt == nil {
		t.Error("expected token to be revoked after Logout")
	}
}

func TestAuthService_Logout_Idempotent(t *testing.T) {
	svc, _, _ := newTestService(t)
	// Logout with a non-existent token should not error
	if err := svc.Logout(context.Background(), "unknown-token"); err != nil {
		t.Errorf("expected idempotent Logout, got error: %v", err)
	}
}

// ── Password reset ────────────────────────────────────────────────────────────

func TestAuthService_RequestPasswordReset_UnknownEmail(t *testing.T) {
	svc, _, _ := newTestService(t)
	// Must not reveal whether email exists
	if err := svc.RequestPasswordReset(context.Background(), "ghost@example.com"); err != nil {
		t.Errorf("expected nil for unknown email, got %v", err)
	}
}

func TestAuthService_ConfirmPasswordReset_HappyPath(t *testing.T) {
	svc, _, mail := newTestService(t)
	ctx := context.Background()

	_, _ = svc.Register(ctx, "alice@example.com", "Password1!")
	_ = svc.RequestPasswordReset(ctx, "alice@example.com")

	// Extract raw token from reset URL: "http://localhost/reset?token=<raw>"
	if mail.lastURL == "" {
		t.Fatal("expected mail to be sent")
	}
	rawToken := mail.lastURL[len("http://localhost/reset?token="):]

	if err := svc.ConfirmPasswordReset(ctx, rawToken, "NewPassword99!"); err != nil {
		t.Fatalf("ConfirmPasswordReset: %v", err)
	}

	// Old password must no longer work
	if _, err := svc.Login(ctx, "alice@example.com", "Password1!", "", ""); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("old password should fail after reset, got: %v", err)
	}

	// New password must work
	if _, err := svc.Login(ctx, "alice@example.com", "NewPassword99!", "", ""); err != nil {
		t.Errorf("new password should work after reset: %v", err)
	}
}

func TestAuthService_ConfirmPasswordReset_InvalidToken(t *testing.T) {
	svc, _, _ := newTestService(t)
	err := svc.ConfirmPasswordReset(context.Background(), "badtoken", "NewPassword99!")
	if !errors.Is(err, domain.ErrTokenInvalid) {
		t.Errorf("expected ErrTokenInvalid, got %v", err)
	}
}

// ── GetUser ───────────────────────────────────────────────────────────────────

func TestAuthService_GetUser_HappyPath(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	created, _ := svc.Register(ctx, "alice@example.com", "Password1!")

	got, err := svc.GetUser(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.Email != "alice@example.com" {
		t.Errorf("email: want alice@example.com, got %s", got.Email)
	}
}

func TestAuthService_GetUser_NotFound(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.GetUser(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

// ── Personal Access Tokens ────────────────────────────────────────────────────

func TestAuthService_CreatePAT_HappyPath(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	user, _ := svc.Register(ctx, "alice@example.com", "Password1!")

	pat, rawToken, err := svc.CreatePAT(ctx, user.ID, "my-mcp-server", 90*24*time.Hour)
	if err != nil {
		t.Fatalf("CreatePAT: %v", err)
	}
	if rawToken == "" {
		t.Error("expected non-empty raw token")
	}
	if !strings.HasPrefix(rawToken, "tbpat_") {
		t.Errorf("expected raw token to start with tbpat_, got %q", rawToken)
	}
	if pat.TokenHash == rawToken {
		t.Error("stored hash must not equal the raw token")
	}
	if pat.Name != "my-mcp-server" {
		t.Errorf("Name: want my-mcp-server, got %s", pat.Name)
	}
	if !pat.IsValid() {
		t.Error("expected freshly created PAT to be valid")
	}
}

func TestAuthService_ListPATs(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	user, _ := svc.Register(ctx, "alice@example.com", "Password1!")
	other, _ := svc.Register(ctx, "bob@example.com", "Password1!")

	_, _, _ = svc.CreatePAT(ctx, user.ID, "token-a", 30*24*time.Hour)
	_, _, _ = svc.CreatePAT(ctx, user.ID, "token-b", 90*24*time.Hour)
	_, _, _ = svc.CreatePAT(ctx, other.ID, "not-alices", 30*24*time.Hour)

	pats, err := svc.ListPATs(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListPATs: %v", err)
	}
	if len(pats) != 2 {
		t.Errorf("expected 2 PATs for user, got %d", len(pats))
	}
}

func TestAuthService_RevokePAT(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	user, _ := svc.Register(ctx, "alice@example.com", "Password1!")
	pat, rawToken, _ := svc.CreatePAT(ctx, user.ID, "my-mcp-server", 90*24*time.Hour)

	if err := svc.RevokePAT(ctx, user.ID, pat.ID); err != nil {
		t.Fatalf("RevokePAT: %v", err)
	}

	if _, err := svc.IntrospectPAT(ctx, rawToken); !errors.Is(err, domain.ErrTokenInvalid) {
		t.Errorf("expected ErrTokenInvalid after revoke, got %v", err)
	}
}

func TestAuthService_RevokePAT_WrongUserIsNoop(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	user, _ := svc.Register(ctx, "alice@example.com", "Password1!")
	attacker, _ := svc.Register(ctx, "mallory@example.com", "Password1!")
	pat, rawToken, _ := svc.CreatePAT(ctx, user.ID, "my-mcp-server", 90*24*time.Hour)

	// Revoking someone else's token must not error (idempotent-style API) but
	// must also not actually revoke it.
	if err := svc.RevokePAT(ctx, attacker.ID, pat.ID); err != nil {
		t.Fatalf("RevokePAT: %v", err)
	}

	if _, err := svc.IntrospectPAT(ctx, rawToken); err != nil {
		t.Errorf("expected token to still be valid, got %v", err)
	}
}

func TestAuthService_IntrospectPAT_HappyPath(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	user, _ := svc.Register(ctx, "alice@example.com", "Password1!")
	_, rawToken, _ := svc.CreatePAT(ctx, user.ID, "my-mcp-server", 90*24*time.Hour)

	got, err := svc.IntrospectPAT(ctx, rawToken)
	if err != nil {
		t.Fatalf("IntrospectPAT: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("IntrospectPAT: want user %s, got %s", user.ID, got.ID)
	}
}

func TestAuthService_IntrospectPAT_UnknownToken(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.IntrospectPAT(context.Background(), "tbpat_doesnotexist")
	if !errors.Is(err, domain.ErrTokenInvalid) {
		t.Errorf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestAuthService_IntrospectPAT_Expired(t *testing.T) {
	svc, repo, _ := newTestService(t)
	ctx := context.Background()
	user, _ := svc.Register(ctx, "alice@example.com", "Password1!")
	_, rawToken, _ := svc.CreatePAT(ctx, user.ID, "my-mcp-server", time.Hour)

	// Backdate expiry directly in the fake store to simulate an expired token.
	for _, pat := range repo.pats {
		if pat.UserID == user.ID {
			pat.ExpiresAt = time.Now().Add(-time.Minute)
		}
	}

	if _, err := svc.IntrospectPAT(ctx, rawToken); !errors.Is(err, domain.ErrTokenInvalid) {
		t.Errorf("expected ErrTokenInvalid for expired PAT, got %v", err)
	}
}
