package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teamboard/services/domain/document/internal/domain"
	"github.com/teamboard/services/domain/document/internal/repository/db"
)

type pgxDBTX struct{ db interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}}

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

// Repo wraps pgxpool and implements domain.Repository.
type Repo struct {
	pool *pgxpool.Pool
	q    *db.Querier
}

func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool, q: db.New(&pgxDBTX{pool})}
}

func (r *Repo) WithTransaction(ctx context.Context, fn func(context.Context, domain.Repository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txRepo := &Repo{pool: r.pool, q: db.New(&pgxDBTX{tx})}
	if err := fn(ctx, txRepo); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ── Helpers ────────────────────────────────────────────────────────────────────

func mapDoc(d *db.Document) *domain.Document {
	if d == nil {
		return nil
	}
	return &domain.Document{
		ID:             d.ID,
		ProjectID:      d.ProjectID,
		Name:           d.Name,
		ContentType:    d.ContentType,
		CurrentVersion: d.CurrentVersion,
		Status:         domain.DocStatus(d.Status),
		CreatedBy:      d.CreatedBy,
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
		DeletedAt:      d.DeletedAt,
	}
}

func mapVersion(v *db.DocumentVersion) *domain.DocumentVersion {
	if v == nil {
		return nil
	}
	return &domain.DocumentVersion{
		ID:             v.ID,
		DocumentID:     v.DocumentID,
		VersionNumber:  v.VersionNumber,
		StorageKey:     v.StorageKey,
		ContentType:    v.ContentType,
		SizeBytes:      v.SizeBytes,
		ChecksumSHA256: v.ChecksumSHA256,
		Status:         domain.VersionStatus(v.Status),
		UploadedBy:     v.UploadedBy,
		UploadedAt:     v.UploadedAt,
		CreatedAt:      v.CreatedAt,
	}
}

func notFound(err error, domErr *domain.Error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domErr
	}
	return err
}

// ── Known entities ─────────────────────────────────────────────────────────────

func (r *Repo) UpsertKnownProject(ctx context.Context, id uuid.UUID) error {
	return r.q.UpsertKnownProject(ctx, id)
}
func (r *Repo) KnownProjectExists(ctx context.Context, id uuid.UUID) (bool, error) {
	return r.q.KnownProjectExists(ctx, id)
}
func (r *Repo) MarkKnownProjectDeleted(ctx context.Context, id uuid.UUID) error {
	return r.q.MarkKnownProjectDeleted(ctx, id)
}
func (r *Repo) UpsertKnownUser(ctx context.Context, id uuid.UUID) error {
	return r.q.UpsertKnownUser(ctx, id)
}
func (r *Repo) MarkKnownUserDeleted(ctx context.Context, id uuid.UUID) error {
	return r.q.MarkKnownUserDeleted(ctx, id)
}

// ── Documents ──────────────────────────────────────────────────────────────────

func (r *Repo) CreateDocument(ctx context.Context, id, projectID uuid.UUID, name, contentType string, createdBy uuid.UUID) (*domain.Document, error) {
	d, err := r.q.CreateDocument(ctx, id, projectID, name, contentType, createdBy)
	if err != nil {
		return nil, err
	}
	return mapDoc(d), nil
}

func (r *Repo) GetDocument(ctx context.Context, id uuid.UUID) (*domain.Document, error) {
	d, err := r.q.GetDocument(ctx, id)
	if err != nil {
		return nil, notFound(err, domain.ErrDocumentNotFound)
	}
	return mapDoc(d), nil
}

func (r *Repo) GetDocumentInternal(ctx context.Context, id uuid.UUID) (*domain.Document, error) {
	d, err := r.q.GetDocumentInternal(ctx, id)
	if err != nil {
		return nil, notFound(err, domain.ErrDocumentNotFound)
	}
	return mapDoc(d), nil
}

func (r *Repo) ListDocumentsByProject(ctx context.Context, projectID uuid.UUID, cursor *time.Time, cursorID *uuid.UUID, limit int) ([]*domain.Document, error) {
	docs, err := r.q.ListDocumentsByProject(ctx, projectID, cursor, cursorID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.Document, len(docs))
	for i, d := range docs {
		result[i] = mapDoc(d)
	}
	return result, nil
}

func (r *Repo) UpdateDocumentName(ctx context.Context, id uuid.UUID, name string) (*domain.Document, error) {
	d, err := r.q.UpdateDocumentName(ctx, id, name)
	if err != nil {
		return nil, notFound(err, domain.ErrDocumentNotFound)
	}
	return mapDoc(d), nil
}

func (r *Repo) ActivateDocument(ctx context.Context, id uuid.UUID, versionNumber int) (*domain.Document, error) {
	d, err := r.q.ActivateDocument(ctx, id, versionNumber)
	if err != nil {
		return nil, notFound(err, domain.ErrDocumentNotFound)
	}
	return mapDoc(d), nil
}

func (r *Repo) SetCurrentVersion(ctx context.Context, id uuid.UUID, versionNumber int) (*domain.Document, error) {
	d, err := r.q.SetCurrentVersion(ctx, id, versionNumber)
	if err != nil {
		return nil, notFound(err, domain.ErrDocumentNotFound)
	}
	return mapDoc(d), nil
}

func (r *Repo) SoftDeleteDocument(ctx context.Context, id uuid.UUID) error {
	return r.q.SoftDeleteDocument(ctx, id)
}

func (r *Repo) SoftDeleteDocumentsByProject(ctx context.Context, projectID uuid.UUID) ([]struct{ ID, ProjectID uuid.UUID }, error) {
	return r.q.SoftDeleteDocumentsByProject(ctx, projectID)
}

func (r *Repo) ListPendingDocumentsOlderThan(ctx context.Context, before time.Time) ([]*domain.Document, error) {
	docs, err := r.q.ListPendingDocumentsOlderThan(ctx, before)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.Document, len(docs))
	for i, d := range docs {
		result[i] = mapDoc(d)
	}
	return result, nil
}

func (r *Repo) ListSoftDeletedOlderThan(ctx context.Context, before time.Time, limit int) ([]*domain.Document, error) {
	docs, err := r.q.ListSoftDeletedOlderThan(ctx, before, limit)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.Document, len(docs))
	for i, d := range docs {
		result[i] = mapDoc(d)
	}
	return result, nil
}

func (r *Repo) HardDeleteDocument(ctx context.Context, id uuid.UUID) error {
	return r.q.HardDeleteDocument(ctx, id)
}

// ── Versions ───────────────────────────────────────────────────────────────────

func (r *Repo) CreateVersion(ctx context.Context, id, documentID uuid.UUID, versionNumber int, storageKey, contentType string, sizeBytes int64, uploadedBy uuid.UUID) (*domain.DocumentVersion, error) {
	v, err := r.q.CreateVersion(ctx, id, documentID, versionNumber, storageKey, contentType, sizeBytes, uploadedBy)
	if err != nil {
		return nil, err
	}
	return mapVersion(v), nil
}

func (r *Repo) GetVersion(ctx context.Context, documentID uuid.UUID, versionNumber int) (*domain.DocumentVersion, error) {
	v, err := r.q.GetVersion(ctx, documentID, versionNumber)
	if err != nil {
		return nil, notFound(err, domain.ErrVersionNotFound)
	}
	return mapVersion(v), nil
}

func (r *Repo) GetCurrentVersion(ctx context.Context, documentID uuid.UUID) (*domain.DocumentVersion, error) {
	v, err := r.q.GetCurrentVersion(ctx, documentID)
	if err != nil {
		return nil, notFound(err, domain.ErrVersionNotFound)
	}
	return mapVersion(v), nil
}

func (r *Repo) ListVersionsByDocument(ctx context.Context, documentID uuid.UUID) ([]*domain.DocumentVersion, error) {
	vers, err := r.q.ListVersionsByDocument(ctx, documentID)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.DocumentVersion, len(vers))
	for i, v := range vers {
		result[i] = mapVersion(v)
	}
	return result, nil
}

func (r *Repo) GetMaxVersionNumber(ctx context.Context, documentID uuid.UUID) (int, error) {
	return r.q.GetMaxVersionNumber(ctx, documentID)
}

func (r *Repo) MarkVersionUploaded(ctx context.Context, id uuid.UUID, checksum *string) (*domain.DocumentVersion, error) {
	v, err := r.q.MarkVersionUploaded(ctx, id, checksum)
	if err != nil {
		return nil, notFound(err, domain.ErrVersionNotFound)
	}
	return mapVersion(v), nil
}

func (r *Repo) MarkVersionFailed(ctx context.Context, id uuid.UUID) error {
	return r.q.MarkVersionFailed(ctx, id)
}

func (r *Repo) ListPendingVersionsOlderThan(ctx context.Context, before time.Time) ([]*domain.DocumentVersion, error) {
	vers, err := r.q.ListPendingVersionsOlderThan(ctx, before)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.DocumentVersion, len(vers))
	for i, v := range vers {
		result[i] = mapVersion(v)
	}
	return result, nil
}

func (r *Repo) ListFailedVersions(ctx context.Context, limit int) ([]*domain.DocumentVersion, error) {
	vers, err := r.q.ListFailedVersions(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.DocumentVersion, len(vers))
	for i, v := range vers {
		result[i] = mapVersion(v)
	}
	return result, nil
}

func (r *Repo) DeleteVersion(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteVersion(ctx, id)
}

// ── Outbox ─────────────────────────────────────────────────────────────────────

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

// ── Idempotency ────────────────────────────────────────────────────────────────

func (r *Repo) WasEventProcessed(ctx context.Context, eventID string) (bool, error) {
	return r.q.WasEventProcessed(ctx, eventID)
}

func (r *Repo) MarkEventProcessed(ctx context.Context, eventID string) error {
	return r.q.MarkEventProcessed(ctx, eventID)
}
