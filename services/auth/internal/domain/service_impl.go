package domain

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
)

// service implements AuthService.
type service struct {
	repo       Repository
	mail       MailSender
	limiter    RateLimiter
	keys       KeyManager
	cfg        ServiceConfig
}

// ServiceConfig holds runtime parameters for the service layer.
type ServiceConfig struct {
	Issuer          string
	Audience        string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	PasswordResetTTL time.Duration
	PasswordResetURL string
	PasswordMinLen  int
	PasswordMaxLen  int
}

// KeyManager provides RSA signing key operations.
type KeyManager interface {
	ActiveKey(ctx context.Context) (*SigningKey, *rsa.PrivateKey, error)
	EnsureActiveKey(ctx context.Context) error
}

// NewAuthService constructs the domain service.
func NewAuthService(repo Repository, mail MailSender, limiter RateLimiter, keys KeyManager, cfg ServiceConfig) AuthService {
	return &service{repo: repo, mail: mail, limiter: limiter, keys: keys, cfg: cfg}
}

var _ AuthService = (*service)(nil)

// Register creates a new user account (UC-1).
func (s *service) Register(ctx context.Context, email, password string) (*User, error) {
	if err := CheckPasswordPolicy(password, s.cfg.PasswordMinLen, s.cfg.PasswordMaxLen); err != nil {
		return nil, err
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	var user *User
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		user, txErr = tx.CreateUser(ctx, uuid.New(), email, hash)
		if txErr != nil {
			return txErr
		}

		payload, _ := json.Marshal(map[string]any{
			"user_id":    user.ID,
			"email":      user.Email,
			"created_at": user.CreatedAt,
		})
		return tx.InsertOutboxEvent(ctx, uuid.New(), user.ID, "user.registered", payload)
	})
	if err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "user registered", "user_id", user.ID)
	return user, nil
}

// Login validates credentials and issues a token pair (UC-2).
func (s *service) Login(ctx context.Context, email, password, ip, userAgent string) (*TokenPair, error) {
	// Rate limit check
	allowed, err := s.limiter.Allow(ctx, "login:"+email, 5, 15*time.Minute)
	if err != nil {
		slog.WarnContext(ctx, "rate limiter error, failing open", "error", err)
	} else if !allowed {
		return nil, ErrRateLimited
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		// Timing-safe: still run Argon2 even for unknown emails
		argon2.IDKey([]byte(password), dummySalt, argon2Iterations, argon2Memory, argon2Parallelism, argon2KeyLen)
		s.recordAttempt(ctx, email, false, ip, userAgent)
		return nil, ErrInvalidCredentials
	}

	if !VerifyPassword(password, user.PasswordHash) {
		s.recordAttempt(ctx, email, false, ip, userAgent)
		return nil, ErrInvalidCredentials
	}

	pair, err := s.issuePair(ctx, user, ip, userAgent)
	if err != nil {
		return nil, err
	}

	_ = s.repo.UpdateLastLogin(ctx, user.ID)
	s.recordAttempt(ctx, email, true, ip, userAgent)
	return pair, nil
}

// Refresh rotates a refresh token and issues a new pair (UC-3).
func (s *service) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	hash := hashToken(refreshToken)

	stored, err := s.repo.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		return nil, ErrTokenInvalid
	}

	if stored.RevokedAt != nil {
		// Reuse detected — revoke all tokens for this user
		_ = s.repo.RevokeAllUserTokens(ctx, stored.UserID)
		slog.WarnContext(ctx, "refresh token reuse detected", "user_id", stored.UserID)
		return nil, ErrTokenRevoked
	}

	if !stored.IsValid() {
		return nil, ErrTokenInvalid
	}

	user, err := s.repo.GetUserByID(ctx, stored.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Revoke old token before issuing new pair
	if err := s.repo.RevokeRefreshToken(ctx, stored.ID, nil); err != nil {
		return nil, fmt.Errorf("revoke old token: %w", err)
	}

	pair, err := s.issuePair(ctx, user, nil, nil)
	if err != nil {
		return nil, err
	}

	// Update replaced_by on old token
	newHash := hashToken(pair.RefreshToken)
	newToken, err := s.repo.GetRefreshTokenByHash(ctx, newHash)
	if err == nil {
		_ = s.repo.RevokeRefreshToken(ctx, stored.ID, &newToken.ID)
	}

	return pair, nil
}

// Logout revokes a refresh token (UC-4).
func (s *service) Logout(ctx context.Context, refreshToken string) error {
	hash := hashToken(refreshToken)
	stored, err := s.repo.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		return nil // idempotent — already gone is fine
	}
	return s.repo.RevokeRefreshToken(ctx, stored.ID, nil)
}

// RequestPasswordReset initiates the reset flow (UC-5).
func (s *service) RequestPasswordReset(ctx context.Context, email string) error {
	allowed, err := s.limiter.Allow(ctx, "pwreset:"+email, 3, time.Hour)
	if err != nil {
		slog.WarnContext(ctx, "rate limiter error, failing open", "error", err)
	} else if !allowed {
		return nil // always return 202-like nil — do not expose rate limit status
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil // don't reveal whether email exists
	}

	token, hash, err := GeneratePasswordResetToken()
	if err != nil {
		return fmt.Errorf("generate reset token: %w", err)
	}

	expiresAt := time.Now().Add(s.cfg.PasswordResetTTL)
	if _, err := s.repo.CreatePasswordResetToken(ctx, uuid.New(), user.ID, hash, expiresAt); err != nil {
		return fmt.Errorf("store reset token: %w", err)
	}

	resetURL := s.cfg.PasswordResetURL + "?token=" + token
	if err := s.mail.SendPasswordReset(ctx, email, resetURL); err != nil {
		slog.ErrorContext(ctx, "failed to send reset email", "error", err, "user_id", user.ID)
		// Don't fail the request — token is stored, user can retry
	}

	return nil
}

// ConfirmPasswordReset completes the reset flow (UC-6).
func (s *service) ConfirmPasswordReset(ctx context.Context, token, newPassword string) error {
	if err := CheckPasswordPolicy(newPassword, s.cfg.PasswordMinLen, s.cfg.PasswordMaxLen); err != nil {
		return err
	}

	hash := hashToken(token)
	resetToken, err := s.repo.GetPasswordResetTokenByHash(ctx, hash)
	if err != nil || !resetToken.IsValid() {
		return ErrTokenInvalid
	}

	newHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	if err := s.repo.UpdatePasswordHash(ctx, resetToken.UserID, newHash); err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if err := s.repo.MarkPasswordResetTokenUsed(ctx, resetToken.ID); err != nil {
		return fmt.Errorf("mark token used: %w", err)
	}
	// Revoke all refresh tokens as a security measure
	return s.repo.RevokeAllUserTokens(ctx, resetToken.UserID)
}

// GetUser returns user info for the authenticated user (UC-7 / GET /auth/me).
func (s *service) GetUser(ctx context.Context, userID uuid.UUID) (*User, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

// ── helpers ────────────────────────────────────────────────────────────────

func (s *service) issuePair(ctx context.Context, user *User, ip, userAgent interface{}) (*TokenPair, error) {
	signingKey, privateKey, err := s.keys.ActiveKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("get signing key: %w", err)
	}

	accessToken, err := IssueAccessToken(user, signingKey.KID, privateKey, s.cfg.Issuer, s.cfg.Audience, s.cfg.AccessTokenTTL)
	if err != nil {
		return nil, err
	}

	rawToken, tokenHash, err := GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	expiresAt := time.Now().Add(s.cfg.RefreshTokenTTL)
	var ipStr, uaStr *string
	if s, ok := ip.(string); ok && s != "" {
		ipStr = &s
	}
	if s, ok := userAgent.(string); ok && s != "" {
		uaStr = &s
	}

	if _, err := s.repo.CreateRefreshToken(ctx, uuid.New(), user.ID, tokenHash, expiresAt, ipStr, uaStr); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: rawToken,
		ExpiresIn:    int(s.cfg.AccessTokenTTL.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

func (s *service) recordAttempt(ctx context.Context, email string, success bool, ip, userAgent interface{}) {
	var ipStr, uaStr *string
	if str, ok := ip.(string); ok && str != "" {
		ipStr = &str
	}
	if str, ok := userAgent.(string); ok && str != "" {
		uaStr = &str
	}
	if err := s.repo.RecordLoginAttempt(ctx, uuid.New(), email, success, ipStr, uaStr); err != nil {
		slog.WarnContext(ctx, "failed to record login attempt", "error", err)
	}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
