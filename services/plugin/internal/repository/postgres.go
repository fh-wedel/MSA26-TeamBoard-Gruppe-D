package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamboard/services/plugin/internal/domain"
)

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type postgresRepo struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) domain.Repository {
	return &postgresRepo{pool: pool}
}

// ── Webhooks ──────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateWebhook(ctx context.Context, wh *domain.Webhook) (*domain.Webhook, error) {
	const q = `INSERT INTO webhooks (id, project_id, target_url, description, secret, event_filter, active, created_by)
               VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, project_id, target_url, description, secret, event_filter, active, created_by, created_at, updated_at`
	row := r.pool.QueryRow(ctx, q,
		wh.ID, wh.ProjectID, wh.TargetURL, wh.Description, wh.Secret, wh.EventFilter, wh.Active, wh.CreatedBy,
	)
	return scanWebhook(row)
}

func (r *postgresRepo) GetWebhook(ctx context.Context, id uuid.UUID) (*domain.Webhook, error) {
	const q = `SELECT id, project_id, target_url, description, secret, event_filter, active, created_by, created_at, updated_at FROM webhooks WHERE id = $1`
	row := r.pool.QueryRow(ctx, q, id)
	wh, err := scanWebhook(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrWebhookNotFound
	}
	return wh, err
}

func (r *postgresRepo) ListWebhooksByProject(ctx context.Context, projectID uuid.UUID) ([]*domain.Webhook, error) {
	const q = `SELECT id, project_id, target_url, description, secret, event_filter, active, created_by, created_at, updated_at
               FROM webhooks WHERE project_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectWebhooks(rows)
}

func (r *postgresRepo) ListActiveWebhooksMatchingEvent(ctx context.Context, projectID uuid.UUID, eventType string) ([]*domain.Webhook, error) {
	const q = `SELECT id, project_id, target_url, description, secret, event_filter, active, created_by, created_at, updated_at
               FROM webhooks
               WHERE project_id = $1 AND active = TRUE
                 AND (
                   event_filter @> ARRAY[$2::TEXT]
                   OR EXISTS (
                     SELECT 1 FROM unnest(event_filter) AS f
                     WHERE f = '*'
                        OR ($2 LIKE replace(f, '.*', '.%') AND f LIKE '%.*')
                   )
                 )`
	rows, err := r.pool.Query(ctx, q, projectID, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectWebhooks(rows)
}

func (r *postgresRepo) UpdateWebhook(ctx context.Context, id uuid.UUID, patch domain.WebhookPatch) (*domain.Webhook, error) {
	const q = `UPDATE webhooks SET
                 target_url   = COALESCE($2, target_url),
                 description  = COALESCE($3, description),
                 event_filter = COALESCE($4, event_filter),
                 active       = COALESCE($5, active),
                 updated_at   = NOW()
               WHERE id = $1
               RETURNING id, project_id, target_url, description, secret, event_filter, active, created_by, created_at, updated_at`
	row := r.pool.QueryRow(ctx, q, id, patch.TargetURL, patch.Description, ptrSlice(patch.EventFilter), patch.Active)
	wh, err := scanWebhook(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrWebhookNotFound
	}
	return wh, err
}

func (r *postgresRepo) UpdateWebhookSecret(ctx context.Context, id uuid.UUID, secret string) (*domain.Webhook, error) {
	const q = `UPDATE webhooks SET secret = $2, updated_at = NOW() WHERE id = $1
               RETURNING id, project_id, target_url, description, secret, event_filter, active, created_by, created_at, updated_at`
	row := r.pool.QueryRow(ctx, q, id, secret)
	wh, err := scanWebhook(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrWebhookNotFound
	}
	return wh, err
}

func (r *postgresRepo) SetWebhookActive(ctx context.Context, id uuid.UUID, active bool) error {
	const q = `UPDATE webhooks SET active = $2, updated_at = NOW() WHERE id = $1`
	tag, err := r.pool.Exec(ctx, q, id, active)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrWebhookNotFound
	}
	return nil
}

func (r *postgresRepo) DeleteWebhook(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM webhooks WHERE id = $1`
	tag, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrWebhookNotFound
	}
	return nil
}

func (r *postgresRepo) DeleteWebhooksByProject(ctx context.Context, projectID uuid.UUID) error {
	const q = `DELETE FROM webhooks WHERE project_id = $1`
	_, err := r.pool.Exec(ctx, q, projectID)
	return err
}

// ── Deliveries ────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateDelivery(ctx context.Context, d *domain.Delivery) (*domain.Delivery, error) {
	payloadJSON, err := json.Marshal(d.Payload)
	if err != nil {
		return nil, err
	}
	const q = `INSERT INTO webhook_deliveries (id, webhook_id, event_id, event_type, payload, status, next_attempt_at)
               VALUES ($1,$2,$3,$4,$5,'pending',NOW())
               RETURNING id, webhook_id, event_id, event_type, payload, status, attempt_count, next_attempt_at,
                         last_response_status, last_response_body, last_error, last_attempted_at,
                         delivered_at, failed_permanently_at, duration_ms, created_at`
	row := r.pool.QueryRow(ctx, q, d.ID, d.WebhookID, d.EventID, d.EventType, payloadJSON)
	return scanDelivery(row)
}

func (r *postgresRepo) PickNextPendingDelivery(ctx context.Context) (*domain.Delivery, error) {
	const q = `SELECT id, webhook_id, event_id, event_type, payload, status, attempt_count, next_attempt_at,
                      last_response_status, last_response_body, last_error, last_attempted_at,
                      delivered_at, failed_permanently_at, duration_ms, created_at
               FROM webhook_deliveries
               WHERE status = 'pending' AND next_attempt_at <= NOW()
               ORDER BY next_attempt_at ASC LIMIT 1
               FOR UPDATE SKIP LOCKED`
	row := r.pool.QueryRow(ctx, q)
	d, err := scanDelivery(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNoPendingDelivery
	}
	return d, err
}

func (r *postgresRepo) GetDelivery(ctx context.Context, id uuid.UUID) (*domain.Delivery, error) {
	const q = `SELECT id, webhook_id, event_id, event_type, payload, status, attempt_count, next_attempt_at,
                      last_response_status, last_response_body, last_error, last_attempted_at,
                      delivered_at, failed_permanently_at, duration_ms, created_at
               FROM webhook_deliveries WHERE id = $1`
	row := r.pool.QueryRow(ctx, q, id)
	d, err := scanDelivery(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrDeliveryNotFound
	}
	return d, err
}

func (r *postgresRepo) ListDeliveriesByWebhook(ctx context.Context, webhookID uuid.UUID, filter domain.DeliveryFilter) ([]*domain.Delivery, error) {
	const q = `SELECT id, webhook_id, event_id, event_type, payload, status, attempt_count, next_attempt_at,
                      last_response_status, last_response_body, last_error, last_attempted_at,
                      delivered_at, failed_permanently_at, duration_ms, created_at
               FROM webhook_deliveries
               WHERE webhook_id = $1
                 AND ($2::TEXT IS NULL OR status = $2)
                 AND ($3::TEXT IS NULL OR event_type = $3)
                 AND ($4::TIMESTAMPTZ IS NULL OR created_at < $4)
               ORDER BY created_at DESC, id DESC
               LIMIT $5`

	var statusStr *string
	if filter.Status != nil {
		s := string(*filter.Status)
		statusStr = &s
	}

	var cursorTime *time.Time
	if filter.Cursor != nil {
		// cursor is stored as RFC3339Nano,uuid
		t, err := time.Parse(time.RFC3339Nano, (*filter.Cursor)[:len(*filter.Cursor)-37])
		if err == nil {
			cursorTime = &t
		}
	}

	rows, err := r.pool.Query(ctx, q, webhookID, statusStr, filter.EventType, cursorTime, filter.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*domain.Delivery
	for rows.Next() {
		d, err := scanDeliveryRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (r *postgresRepo) MarkDeliveryDelivered(ctx context.Context, id uuid.UUID, statusCode int, body string, durationMs int) error {
	const q = `UPDATE webhook_deliveries SET
                 status = 'delivered', attempt_count = attempt_count + 1,
                 last_response_status = $2, last_response_body = $3,
                 last_attempted_at = NOW(), delivered_at = NOW(), duration_ms = $4
               WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id, statusCode, body, durationMs)
	return err
}

func (r *postgresRepo) RescheduleDelivery(ctx context.Context, id uuid.UUID, nextAt time.Time, statusCode *int, body, errMsg *string, durationMs int) error {
	const q = `UPDATE webhook_deliveries SET
                 status = 'pending', attempt_count = attempt_count + 1,
                 next_attempt_at = $2,
                 last_response_status = $3, last_response_body = $4,
                 last_error = $5, last_attempted_at = NOW(), duration_ms = $6
               WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id, nextAt, statusCode, body, errMsg, durationMs)
	return err
}

func (r *postgresRepo) MarkDeliveryDead(ctx context.Context, id uuid.UUID, reason string) error {
	const q = `UPDATE webhook_deliveries SET
                 status = 'dead', attempt_count = attempt_count + 1,
                 last_error = $2, last_attempted_at = NOW(), failed_permanently_at = NOW()
               WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id, reason)
	return err
}

func (r *postgresRepo) MarkPendingDeliveriesDeadByWebhook(ctx context.Context, webhookID uuid.UUID) error {
	const q = `UPDATE webhook_deliveries SET
                 status = 'dead', last_error = 'webhook deleted', failed_permanently_at = NOW()
               WHERE webhook_id = $1 AND status = 'pending'`
	_, err := r.pool.Exec(ctx, q, webhookID)
	return err
}

func (r *postgresRepo) DeleteOldDeliveries(ctx context.Context, before time.Time) (int64, error) {
	const q = `DELETE FROM webhook_deliveries WHERE created_at < $1 AND status IN ('delivered','dead')`
	tag, err := r.pool.Exec(ctx, q, before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ── Known projects ────────────────────────────────────────────────────────────

func (r *postgresRepo) UpsertKnownProject(ctx context.Context, id uuid.UUID) error {
	const q = `INSERT INTO known_projects (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`
	_, err := r.pool.Exec(ctx, q, id)
	return err
}

func (r *postgresRepo) KnownProjectExists(ctx context.Context, id uuid.UUID) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM known_projects WHERE id = $1 AND deleted_at IS NULL)`
	var exists bool
	err := r.pool.QueryRow(ctx, q, id).Scan(&exists)
	return exists, err
}

func (r *postgresRepo) MarkKnownProjectDeleted(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE known_projects SET deleted_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id)
	return err
}

// ── Outbox ────────────────────────────────────────────────────────────────────

func (r *postgresRepo) InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error {
	const q = `INSERT INTO outbox (id, aggregate_id, event_type, payload) VALUES ($1,$2,$3,$4)`
	_, err := r.pool.Exec(ctx, q, id, aggregateID, eventType, payload)
	return err
}

func (r *postgresRepo) GetUnpublishedEvents(ctx context.Context, limit int32) ([]*domain.OutboxEvent, error) {
	const q = `SELECT id, aggregate_id, event_type, payload, occurred_at, published_at
               FROM outbox WHERE published_at IS NULL ORDER BY occurred_at ASC LIMIT $1`
	rows, err := r.pool.Query(ctx, q, limit)
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
	_, err := r.pool.Exec(ctx, q, id)
	return err
}

// ── Processed events ──────────────────────────────────────────────────────────

func (r *postgresRepo) WasEventProcessed(ctx context.Context, eventID string) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1)`
	var exists bool
	err := r.pool.QueryRow(ctx, q, eventID).Scan(&exists)
	return exists, err
}

func (r *postgresRepo) MarkEventProcessed(ctx context.Context, eventID string) error {
	const q = `INSERT INTO processed_events (event_id) VALUES ($1) ON CONFLICT DO NOTHING`
	_, err := r.pool.Exec(ctx, q, eventID)
	return err
}

// ── Scan helpers ──────────────────────────────────────────────────────────────

func scanWebhook(row pgx.Row) (*domain.Webhook, error) {
	wh := &domain.Webhook{}
	err := row.Scan(
		&wh.ID, &wh.ProjectID, &wh.TargetURL, &wh.Description,
		&wh.Secret, &wh.EventFilter, &wh.Active, &wh.CreatedBy,
		&wh.CreatedAt, &wh.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return wh, nil
}

func collectWebhooks(rows pgx.Rows) ([]*domain.Webhook, error) {
	var result []*domain.Webhook
	for rows.Next() {
		wh := &domain.Webhook{}
		if err := rows.Scan(
			&wh.ID, &wh.ProjectID, &wh.TargetURL, &wh.Description,
			&wh.Secret, &wh.EventFilter, &wh.Active, &wh.CreatedBy,
			&wh.CreatedAt, &wh.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, wh)
	}
	return result, rows.Err()
}

func scanDelivery(row pgx.Row) (*domain.Delivery, error) {
	d := &domain.Delivery{}
	var payloadRaw []byte
	err := row.Scan(
		&d.ID, &d.WebhookID, &d.EventID, &d.EventType, &payloadRaw,
		&d.Status, &d.AttemptCount, &d.NextAttemptAt,
		&d.LastResponseStatus, &d.LastResponseBody, &d.LastError, &d.LastAttemptedAt,
		&d.DeliveredAt, &d.FailedPermanentlyAt, &d.DurationMs, &d.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(payloadRaw, &d.Payload); err != nil {
		return nil, err
	}
	return d, nil
}

func scanDeliveryRow(rows pgx.Rows) (*domain.Delivery, error) {
	d := &domain.Delivery{}
	var payloadRaw []byte
	err := rows.Scan(
		&d.ID, &d.WebhookID, &d.EventID, &d.EventType, &payloadRaw,
		&d.Status, &d.AttemptCount, &d.NextAttemptAt,
		&d.LastResponseStatus, &d.LastResponseBody, &d.LastError, &d.LastAttemptedAt,
		&d.DeliveredAt, &d.FailedPermanentlyAt, &d.DurationMs, &d.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(payloadRaw, &d.Payload); err != nil {
		return nil, err
	}
	return d, nil
}

func ptrSlice(p *[]string) *[]string { return p }
