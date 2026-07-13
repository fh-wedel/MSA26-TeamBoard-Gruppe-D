package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teamboard/services/auth/internal/domain"
	"github.com/teamboard/services/auth/internal/repository/db"
)

type postgresRepo struct {
	pool *pgxpool.Pool
	q    db.Querier
}

// New creates a Repository backed by a pgxpool.Pool.
func New(pool *pgxpool.Pool) domain.Repository {
	return &postgresRepo{pool: pool, q: db.NewFromPool(pool)}
}

var _ domain.Repository = (*postgresRepo)(nil)

// ── Users ──────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateUser(ctx context.Context, id uuid.UUID, email, passwordHash string) (*domain.User, error) {
	row, err := r.q.CreateUser(ctx, db.CreateUserParams{
		ID:           id,
		Email:        email,
		PasswordHash: passwordHash,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, domain.ErrEmailTaken
		}
		return nil, err
	}
	return mapUser(row), nil
}

func (r *postgresRepo) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return mapUser(row), nil
}

func (r *postgresRepo) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return mapUser(row), nil
}

func (r *postgresRepo) UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error {
	return r.q.UpdatePasswordHash(ctx, db.UpdatePasswordHashParams{ID: id, PasswordHash: hash})
}

func (r *postgresRepo) UpdateLastLogin(ctx context.Context, id uuid.UUID) error {
	return r.q.UpdateLastLogin(ctx, id)
}

// ── Refresh tokens ──────────────────────────────────────────────────────────

func (r *postgresRepo) CreateRefreshToken(ctx context.Context, id, userID uuid.UUID, tokenHash string, expiresAt time.Time, ip, userAgent *string) (*domain.RefreshToken, error) {
	row, err := r.q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		ID:        id,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		IPAddress: ip,
		UserAgent: userAgent,
	})
	if err != nil {
		return nil, err
	}
	return mapRefreshToken(row), nil
}

func (r *postgresRepo) GetRefreshTokenByHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	row, err := r.q.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTokenInvalid
		}
		return nil, err
	}
	return mapRefreshToken(row), nil
}

func (r *postgresRepo) RevokeRefreshToken(ctx context.Context, id uuid.UUID, replacedBy *uuid.UUID) error {
	return r.q.RevokeRefreshToken(ctx, db.RevokeRefreshTokenParams{ID: id, ReplacedBy: replacedBy})
}

func (r *postgresRepo) RevokeAllUserTokens(ctx context.Context, userID uuid.UUID) error {
	return r.q.RevokeAllUserTokens(ctx, userID)
}

// ── Personal access tokens ───────────────────────────────────────────────────

func (r *postgresRepo) CreatePersonalAccessToken(ctx context.Context, id, userID uuid.UUID, name, tokenHash, tokenPrefix string, expiresAt time.Time) (*domain.PersonalAccessToken, error) {
	row, err := r.q.CreatePersonalAccessToken(ctx, db.CreatePersonalAccessTokenParams{
		ID:          id,
		UserID:      userID,
		Name:        name,
		TokenHash:   tokenHash,
		TokenPrefix: tokenPrefix,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return nil, err
	}
	return mapPersonalAccessToken(row), nil
}

func (r *postgresRepo) GetPersonalAccessTokenByHash(ctx context.Context, hash string) (*domain.PersonalAccessToken, error) {
	row, err := r.q.GetPersonalAccessTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTokenInvalid
		}
		return nil, err
	}
	return mapPersonalAccessToken(row), nil
}

func (r *postgresRepo) ListPersonalAccessTokensByUser(ctx context.Context, userID uuid.UUID) ([]*domain.PersonalAccessToken, error) {
	rows, err := r.q.ListPersonalAccessTokensByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	tokens := make([]*domain.PersonalAccessToken, len(rows))
	for i, row := range rows {
		tokens[i] = mapPersonalAccessToken(row)
	}
	return tokens, nil
}

func (r *postgresRepo) RevokePersonalAccessToken(ctx context.Context, id, userID uuid.UUID) error {
	return r.q.RevokePersonalAccessToken(ctx, db.RevokePersonalAccessTokenParams{ID: id, UserID: userID})
}

func (r *postgresRepo) TouchPersonalAccessTokenLastUsed(ctx context.Context, id uuid.UUID) error {
	return r.q.TouchPersonalAccessTokenLastUsed(ctx, id)
}

// ── Password reset ──────────────────────────────────────────────────────────

func (r *postgresRepo) CreatePasswordResetToken(ctx context.Context, id, userID uuid.UUID, hash string, expiresAt time.Time) (*domain.PasswordResetToken, error) {
	row, err := r.q.CreatePasswordResetToken(ctx, db.CreatePasswordResetTokenParams{
		ID:        id,
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, err
	}
	return mapPasswordResetToken(row), nil
}

func (r *postgresRepo) GetPasswordResetTokenByHash(ctx context.Context, hash string) (*domain.PasswordResetToken, error) {
	row, err := r.q.GetPasswordResetTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTokenInvalid
		}
		return nil, err
	}
	return mapPasswordResetToken(row), nil
}

func (r *postgresRepo) MarkPasswordResetTokenUsed(ctx context.Context, id uuid.UUID) error {
	return r.q.MarkPasswordResetTokenUsed(ctx, id)
}

// ── Signing keys ──────────────────────────────────────────────────────────

func (r *postgresRepo) GetActiveSigningKey(ctx context.Context) (*domain.SigningKey, error) {
	row, err := r.q.GetActiveSigningKey(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrSigningKeyNotFound
		}
		return nil, err
	}
	return mapSigningKey(row), nil
}

func (r *postgresRepo) ListValidatingSigningKeys(ctx context.Context) ([]*domain.SigningKey, error) {
	rows, err := r.q.ListValidatingSigningKeys(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]*domain.SigningKey, len(rows))
	for i, row := range rows {
		keys[i] = mapSigningKey(row)
	}
	return keys, nil
}

func (r *postgresRepo) CreateSigningKey(ctx context.Context, id uuid.UUID, kid, algorithm, privatePEM, publicPEM string) (*domain.SigningKey, error) {
	row, err := r.q.CreateSigningKey(ctx, db.CreateSigningKeyParams{
		ID:            id,
		Kid:           kid,
		Algorithm:     algorithm,
		PrivateKeyPem: privatePEM,
		PublicKeyPem:  publicPEM,
	})
	if err != nil {
		return nil, err
	}
	return mapSigningKey(row), nil
}

func (r *postgresRepo) RetireSigningKey(ctx context.Context, id uuid.UUID) error {
	return r.q.RetireSigningKey(ctx, id)
}

func (r *postgresRepo) DeleteRetiredSigningKeys(ctx context.Context) error {
	return r.q.DeleteRetiredSigningKeys(ctx)
}

// ── Login attempts ──────────────────────────────────────────────────────────

func (r *postgresRepo) RecordLoginAttempt(ctx context.Context, id uuid.UUID, email string, success bool, ip, userAgent *string) error {
	return r.q.RecordLoginAttempt(ctx, db.RecordLoginAttemptParams{
		ID:        id,
		Email:     email,
		Success:   success,
		IPAddress: ip,
		UserAgent: userAgent,
	})
}

func (r *postgresRepo) CountRecentFailedAttempts(ctx context.Context, email string) (int64, error) {
	return r.q.CountRecentFailedAttempts(ctx, email)
}

// ── Outbox ──────────────────────────────────────────────────────────────────

func (r *postgresRepo) InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error {
	return r.q.InsertOutboxEvent(ctx, db.InsertOutboxEventParams{
		ID:          id,
		AggregateID: aggregateID,
		EventType:   eventType,
		Payload:     payload,
	})
}

func (r *postgresRepo) WithTransaction(ctx context.Context, fn func(context.Context, domain.Repository) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	txRepo := &postgresRepo{pool: r.pool, q: db.New(tx)}
	if err := fn(ctx, txRepo); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// ── Mapping helpers ──────────────────────────────────────────────────────────

func mapUser(u db.User) *domain.User {
	return &domain.User{
		ID:           u.ID,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
		DeletedAt:    u.DeletedAt,
		LastLoginAt:  u.LastLoginAt,
	}
}

func mapRefreshToken(t db.RefreshToken) *domain.RefreshToken {
	return &domain.RefreshToken{
		ID:         t.ID,
		UserID:     t.UserID,
		TokenHash:  t.TokenHash,
		IssuedAt:   t.IssuedAt,
		ExpiresAt:  t.ExpiresAt,
		RevokedAt:  t.RevokedAt,
		ReplacedBy: t.ReplacedBy,
	}
}

func mapPasswordResetToken(t db.PasswordResetToken) *domain.PasswordResetToken {
	return &domain.PasswordResetToken{
		ID:        t.ID,
		UserID:    t.UserID,
		TokenHash: t.TokenHash,
		IssuedAt:  t.IssuedAt,
		ExpiresAt: t.ExpiresAt,
		UsedAt:    t.UsedAt,
	}
}

func mapPersonalAccessToken(t db.PersonalAccessToken) *domain.PersonalAccessToken {
	return &domain.PersonalAccessToken{
		ID:          t.ID,
		UserID:      t.UserID,
		Name:        t.Name,
		TokenHash:   t.TokenHash,
		TokenPrefix: t.TokenPrefix,
		CreatedAt:   t.CreatedAt,
		ExpiresAt:   t.ExpiresAt,
		RevokedAt:   t.RevokedAt,
		LastUsedAt:  t.LastUsedAt,
	}
}

func mapSigningKey(k db.SigningKey) *domain.SigningKey {
	return &domain.SigningKey{
		ID:            k.ID,
		KID:           k.Kid,
		Algorithm:     k.Algorithm,
		PrivateKeyPEM: k.PrivateKeyPem,
		PublicKeyPEM:  k.PublicKeyPem,
		CreatedAt:     k.CreatedAt,
		ActivatedAt:   k.ActivatedAt,
		RetiredAt:     k.RetiredAt,
		DeletedAt:     k.DeletedAt,
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
