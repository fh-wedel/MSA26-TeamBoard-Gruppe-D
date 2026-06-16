package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teamboard/services/boardregistry/internal/domain"
)

// DBTX is satisfied by both *pgxpool.Pool and pgx.Tx.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type postgresRepo struct {
	db   DBTX
	pool *pgxpool.Pool // nil when this repo is bound to a transaction
}

// New constructs a pool-backed repository.
func New(pool *pgxpool.Pool) domain.Repository {
	return &postgresRepo{db: pool, pool: pool}
}

func (r *postgresRepo) WithTransaction(ctx context.Context, fn func(context.Context, domain.Repository) error) error {
	if r.pool == nil {
		// already inside a transaction
		return fn(ctx, r)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	txRepo := &postgresRepo{db: tx, pool: nil}
	if err := fn(ctx, txRepo); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// ── Board types ─────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateBoardType(ctx context.Context, def *domain.BoardTypeDef) (*domain.BoardTypeDef, error) {
	cols, _ := json.Marshal(def.DefaultColumns)
	cfg, _ := json.Marshal(def.DefaultConfig)
	schema, _ := json.Marshal(def.ConfigSchema)
	pres, _ := json.Marshal(def.Presentation)
	const q = `INSERT INTO board_types
	             (id, type, display_name, icon, default_columns, default_config, config_schema, presentation, built_in, created_by)
	           VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	           RETURNING id, type, display_name, icon, default_columns, default_config, config_schema, presentation, built_in, created_by, created_at, updated_at`
	row := r.db.QueryRow(ctx, q,
		def.ID, def.Type, def.DisplayName, def.Icon, cols, cfg, schema, pres, def.BuiltIn, def.CreatedBy)
	return scanBoardType(row)
}

func (r *postgresRepo) GetBoardType(ctx context.Context, typ string) (*domain.BoardTypeDef, error) {
	const q = `SELECT id, type, display_name, icon, default_columns, default_config, config_schema, presentation, built_in, created_by, created_at, updated_at
	           FROM board_types WHERE type = $1`
	def, err := scanBoardType(r.db.QueryRow(ctx, q, typ))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return def, err
}

func (r *postgresRepo) ListBoardTypes(ctx context.Context) ([]*domain.BoardTypeDef, error) {
	const q = `SELECT id, type, display_name, icon, default_columns, default_config, config_schema, presentation, built_in, created_by, created_at, updated_at
	           FROM board_types ORDER BY built_in DESC, type ASC`
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*domain.BoardTypeDef
	for rows.Next() {
		def, err := scanBoardTypeRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, def)
	}
	return result, rows.Err()
}

func (r *postgresRepo) UpdateBoardType(ctx context.Context, def *domain.BoardTypeDef) (*domain.BoardTypeDef, error) {
	cols, _ := json.Marshal(def.DefaultColumns)
	cfg, _ := json.Marshal(def.DefaultConfig)
	schema, _ := json.Marshal(def.ConfigSchema)
	pres, _ := json.Marshal(def.Presentation)
	const q = `UPDATE board_types SET
	             display_name = $2, icon = $3, default_columns = $4,
	             default_config = $5, config_schema = $6, presentation = $7, updated_at = NOW()
	           WHERE type = $1
	           RETURNING id, type, display_name, icon, default_columns, default_config, config_schema, presentation, built_in, created_by, created_at, updated_at`
	def2, err := scanBoardType(r.db.QueryRow(ctx, q, def.Type, def.DisplayName, def.Icon, cols, cfg, schema, pres))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return def2, err
}

func (r *postgresRepo) DeleteBoardType(ctx context.Context, typ string) error {
	const q = `DELETE FROM board_types WHERE type = $1`
	tag, err := r.db.Exec(ctx, q, typ)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ── Outbox ──────────────────────────────────────────────────────────────────────

func (r *postgresRepo) InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error {
	const q = `INSERT INTO outbox (id, aggregate_id, event_type, payload) VALUES ($1,$2,$3,$4)`
	_, err := r.db.Exec(ctx, q, id, aggregateID, eventType, payload)
	return err
}

func (r *postgresRepo) GetUnpublishedEvents(ctx context.Context, limit int32) ([]*domain.OutboxEvent, error) {
	const q = `SELECT id, aggregate_id, event_type, payload, occurred_at, published_at
	           FROM outbox WHERE published_at IS NULL ORDER BY occurred_at ASC LIMIT $1`
	rows, err := r.db.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*domain.OutboxEvent
	for rows.Next() {
		e := &domain.OutboxEvent{}
		if err := rows.Scan(&e.ID, &e.AggregateID, &e.EventType, &e.Payload, &e.OccurredAt, &e.PublishedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (r *postgresRepo) MarkEventPublished(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE outbox SET published_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(ctx, q, id)
	return err
}

// ── Scan helpers ──────────────────────────────────────────────────────────────

func scanBoardType(row pgx.Row) (*domain.BoardTypeDef, error) {
	def := &domain.BoardTypeDef{}
	var cols, cfg, schema, pres []byte
	err := row.Scan(&def.ID, &def.Type, &def.DisplayName, &def.Icon,
		&cols, &cfg, &schema, &pres, &def.BuiltIn, &def.CreatedBy, &def.CreatedAt, &def.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := unmarshalDef(def, cols, cfg, schema, pres); err != nil {
		return nil, err
	}
	return def, nil
}

func scanBoardTypeRow(rows pgx.Rows) (*domain.BoardTypeDef, error) {
	def := &domain.BoardTypeDef{}
	var cols, cfg, schema, pres []byte
	err := rows.Scan(&def.ID, &def.Type, &def.DisplayName, &def.Icon,
		&cols, &cfg, &schema, &pres, &def.BuiltIn, &def.CreatedBy, &def.CreatedAt, &def.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := unmarshalDef(def, cols, cfg, schema, pres); err != nil {
		return nil, err
	}
	return def, nil
}

func unmarshalDef(def *domain.BoardTypeDef, cols, cfg, schema, pres []byte) error {
	if len(cols) > 0 {
		if err := json.Unmarshal(cols, &def.DefaultColumns); err != nil {
			return err
		}
	}
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &def.DefaultConfig); err != nil {
			return err
		}
	}
	if len(schema) > 0 {
		if err := json.Unmarshal(schema, &def.ConfigSchema); err != nil {
			return err
		}
	}
	if len(pres) > 0 {
		if err := json.Unmarshal(pres, &def.Presentation); err != nil {
			return err
		}
	}
	return nil
}
