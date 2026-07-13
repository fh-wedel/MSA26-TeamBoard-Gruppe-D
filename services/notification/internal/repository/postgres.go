package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teamboard/services/notification/internal/domain"
	"github.com/teamboard/services/notification/internal/repository/db"
)

type pgxDBTX struct {
	db interface {
		Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
		Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
		QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	}
}

func (p *pgxDBTX) Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error) {
	tag, err := p.db.Exec(ctx, sql, args...)
	return tag, err
}
func (p *pgxDBTX) Query(ctx context.Context, sql string, args ...any) (db.Rows, error) {
	return p.db.Query(ctx, sql, args...)
}
func (p *pgxDBTX) QueryRow(ctx context.Context, sql string, args ...any) db.Row {
	return p.db.QueryRow(ctx, sql, args...)
}

type Repo struct {
	pool *pgxpool.Pool
	q    *db.Querier
}

func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool, q: db.New(&pgxDBTX{pool})}
}

func notFound(err error, domErr *domain.Error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domErr
	}
	return err
}

func mapNotification(n *db.Notification) (*domain.Notification, error) {
	if n == nil {
		return nil, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(n.Payload, &payload); err != nil {
		payload = map[string]any{}
	}
	return &domain.Notification{
		ID:            n.ID,
		UserID:        n.UserID,
		Type:          domain.NotificationType(n.Type),
		Payload:       payload,
		ProjectID:     n.ProjectID,
		SourceEventID: n.SourceEventID,
		CreatedAt:     n.CreatedAt,
		ReadAt:        n.ReadAt,
	}, nil
}

func (r *Repo) CreateNotification(ctx context.Context, n *domain.Notification) (*domain.Notification, error) {
	payloadJSON, err := json.Marshal(n.Payload)
	if err != nil {
		return nil, err
	}
	dbN, err := r.q.CreateNotification(ctx, n.ID, n.UserID, string(n.Type), string(payloadJSON), n.ProjectID, n.SourceEventID)
	if err != nil {
		// ON CONFLICT DO NOTHING returns ErrNoRows when ignored
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // idempotent duplicate
		}
		return nil, err
	}
	return mapNotification(dbN)
}

func (r *Repo) GetNotification(ctx context.Context, id, userID uuid.UUID) (*domain.Notification, error) {
	n, err := r.q.GetNotification(ctx, id, userID)
	if err != nil {
		return nil, notFound(err, domain.ErrNotificationNotFound)
	}
	return mapNotification(n)
}

func (r *Repo) ListNotifications(ctx context.Context, userID uuid.UUID, filter domain.ListFilter) ([]*domain.Notification, error) {
	var (
		cursorTime *time.Time
		cursorID   *uuid.UUID
		typStr     *string
	)
	if filter.Type != nil {
		s := string(*filter.Type)
		typStr = &s
	}
	if filter.Cursor != nil {
		// Cursor is "time,uuid" (parsed in service layer)
		parts := strings.SplitN(*filter.Cursor, ",", 2)
		if len(parts) == 2 {
			t, err := time.Parse(time.RFC3339Nano, parts[0])
			if err == nil {
				cursorTime = &t
				id, err := uuid.Parse(parts[1])
				if err == nil {
					cursorID = &id
				}
			}
		}
	}

	ns, err := r.q.ListNotifications(ctx, userID, filter.UnreadOnly, typStr, cursorTime, cursorID, filter.Limit)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.Notification, 0, len(ns))
	for _, n := range ns {
		dn, err := mapNotification(n)
		if err != nil {
			return nil, err
		}
		result = append(result, dn)
	}
	return result, nil
}

func (r *Repo) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	return r.q.CountUnread(ctx, userID)
}

func (r *Repo) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	return r.q.MarkRead(ctx, id, userID)
}

func (r *Repo) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	return r.q.MarkAllRead(ctx, userID)
}

func (r *Repo) DeleteNotification(ctx context.Context, id, userID uuid.UUID) error {
	return r.q.DeleteNotification(ctx, id, userID)
}

func (r *Repo) DeleteNotificationsByProject(ctx context.Context, projectID uuid.UUID) error {
	return r.q.DeleteNotificationsByProject(ctx, projectID)
}

func (r *Repo) DeleteOldNotifications(ctx context.Context, before time.Time) (int64, error) {
	return r.q.DeleteOldNotifications(ctx, before)
}

func (r *Repo) InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error {
	return r.q.InsertOutboxEvent(ctx, id, aggregateID, eventType, payload)
}

func (r *Repo) GetUnpublishedEvents(ctx context.Context, limit int32) ([]*domain.OutboxEvent, error) {
	evts, err := r.q.GetUnpublishedEvents(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.OutboxEvent, len(evts))
	for i, e := range evts {
		result[i] = &domain.OutboxEvent{
			ID:          e.ID,
			AggregateID: e.AggregateID,
			EventType:   e.EventType,
			Payload:     e.Payload,
			OccurredAt:  e.OccurredAt,
			PublishedAt: e.PublishedAt,
		}
	}
	return result, nil
}

func (r *Repo) MarkEventPublished(ctx context.Context, id uuid.UUID) error {
	return r.q.MarkEventPublished(ctx, id)
}

func (r *Repo) WasEventProcessed(ctx context.Context, eventID string) (bool, error) {
	return r.q.WasEventProcessed(ctx, eventID)
}

func (r *Repo) MarkEventProcessed(ctx context.Context, eventID string) error {
	return r.q.MarkEventProcessed(ctx, eventID)
}
