package db

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error)
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
}

type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close()
	Err() error
}

type Row interface {
	Scan(dest ...any) error
}

type Querier struct{ db DBTX }

func New(db DBTX) *Querier { return &Querier{db: db} }

// ── Notifications ──────────────────────────────────────────────────────────────

func (q *Querier) CreateNotification(ctx context.Context, id, userID uuid.UUID, typ, payload string, projectID *uuid.UUID, sourceEventID *string) (*Notification, error) {
	row := q.db.QueryRow(ctx,
		`INSERT INTO notifications (id, user_id, type, payload, project_id, source_event_id)
		 VALUES ($1, $2, $3, $4::jsonb, $5, $6)
		 ON CONFLICT (user_id, source_event_id) WHERE source_event_id IS NOT NULL DO NOTHING
		 RETURNING id, user_id, type, payload, project_id, source_event_id, created_at, read_at`,
		id, userID, typ, payload, projectID, sourceEventID)
	return scanNotification(row)
}

func (q *Querier) GetNotification(ctx context.Context, id, userID uuid.UUID) (*Notification, error) {
	row := q.db.QueryRow(ctx,
		`SELECT id, user_id, type, payload, project_id, source_event_id, created_at, read_at
		 FROM notifications WHERE id = $1 AND user_id = $2`, id, userID)
	return scanNotification(row)
}

func (q *Querier) ListNotifications(ctx context.Context, userID uuid.UUID, unreadOnly bool, typ *string, cursorTime *time.Time, cursorID *uuid.UUID, limit int) ([]*Notification, error) {
	var (
		rows Rows
		err  error
	)
	if cursorTime != nil && cursorID != nil {
		rows, err = q.db.Query(ctx,
			`SELECT id, user_id, type, payload, project_id, source_event_id, created_at, read_at
			 FROM notifications
			 WHERE user_id = $1
			   AND ($2::BOOLEAN = FALSE OR read_at IS NULL)
			   AND ($3::TEXT IS NULL OR type = $3)
			   AND (created_at, id) < ($4, $5)
			 ORDER BY created_at DESC, id DESC LIMIT $6`,
			userID, unreadOnly, typ, cursorTime, cursorID, limit)
	} else {
		rows, err = q.db.Query(ctx,
			`SELECT id, user_id, type, payload, project_id, source_event_id, created_at, read_at
			 FROM notifications
			 WHERE user_id = $1
			   AND ($2::BOOLEAN = FALSE OR read_at IS NULL)
			   AND ($3::TEXT IS NULL OR type = $3)
			 ORDER BY created_at DESC, id DESC LIMIT $4`,
			userID, unreadOnly, typ, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotifications(rows)
}

func (q *Querier) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := q.db.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

func (q *Querier) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	_, err := q.db.Exec(ctx, `UPDATE notifications SET read_at = NOW() WHERE id = $1 AND user_id = $2 AND read_at IS NULL`, id, userID)
	return err
}

func (q *Querier) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	_, err := q.db.Exec(ctx, `UPDATE notifications SET read_at = NOW() WHERE user_id = $1 AND read_at IS NULL`, userID)
	return err
}

func (q *Querier) DeleteNotification(ctx context.Context, id, userID uuid.UUID) error {
	_, err := q.db.Exec(ctx, `DELETE FROM notifications WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

func (q *Querier) DeleteNotificationsByProject(ctx context.Context, projectID uuid.UUID) error {
	_, err := q.db.Exec(ctx, `DELETE FROM notifications WHERE project_id = $1`, projectID)
	return err
}

func (q *Querier) DeleteOldNotifications(ctx context.Context, before time.Time) (int64, error) {
	tag, err := q.db.Exec(ctx, `DELETE FROM notifications WHERE created_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ── Outbox ─────────────────────────────────────────────────────────────────────

func (q *Querier) InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error {
	_, err := q.db.Exec(ctx,
		`INSERT INTO outbox (id, aggregate_id, event_type, payload) VALUES ($1, $2, $3, $4)`,
		id, aggregateID, eventType, payload)
	return err
}

func (q *Querier) GetUnpublishedEvents(ctx context.Context, limit int32) ([]*OutboxEvent, error) {
	rows, err := q.db.Query(ctx,
		`SELECT id, aggregate_id, event_type, payload, occurred_at, published_at
		 FROM outbox WHERE published_at IS NULL ORDER BY occurred_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var evts []*OutboxEvent
	for rows.Next() {
		e := &OutboxEvent{}
		if err := rows.Scan(&e.ID, &e.AggregateID, &e.EventType, &e.Payload, &e.OccurredAt, &e.PublishedAt); err != nil {
			return nil, err
		}
		evts = append(evts, e)
	}
	return evts, rows.Err()
}

func (q *Querier) MarkEventPublished(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, `UPDATE outbox SET published_at = NOW() WHERE id = $1`, id)
	return err
}

// ── Idempotency ────────────────────────────────────────────────────────────────

func (q *Querier) WasEventProcessed(ctx context.Context, eventID string) (bool, error) {
	var exists bool
	err := q.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1)`, eventID).Scan(&exists)
	return exists, err
}

func (q *Querier) MarkEventProcessed(ctx context.Context, eventID string) error {
	_, err := q.db.Exec(ctx, `INSERT INTO processed_events (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, eventID)
	return err
}

// ── Scan helpers ───────────────────────────────────────────────────────────────

func scanNotification(row Row) (*Notification, error) {
	n := &Notification{}
	err := row.Scan(&n.ID, &n.UserID, &n.Type, &n.Payload, &n.ProjectID, &n.SourceEventID, &n.CreatedAt, &n.ReadAt)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func scanNotifications(rows Rows) ([]*Notification, error) {
	var ns []*Notification
	for rows.Next() {
		n := &Notification{}
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Payload, &n.ProjectID, &n.SourceEventID, &n.CreatedAt, &n.ReadAt); err != nil {
			return nil, err
		}
		ns = append(ns, n)
	}
	return ns, rows.Err()
}
