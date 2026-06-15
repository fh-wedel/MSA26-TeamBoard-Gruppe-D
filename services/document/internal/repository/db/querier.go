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

// ── Known entities ─────────────────────────────────────────────────────────────

func (q *Querier) UpsertKnownProject(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx,
		`INSERT INTO known_projects (id) VALUES ($1) ON CONFLICT (id) DO UPDATE SET deleted_at = NULL`, id)
	return err
}

func (q *Querier) KnownProjectExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := q.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM known_projects WHERE id = $1 AND deleted_at IS NULL)`, id).Scan(&exists)
	return exists, err
}

func (q *Querier) MarkKnownProjectDeleted(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, `UPDATE known_projects SET deleted_at = NOW() WHERE id = $1`, id)
	return err
}

func (q *Querier) UpsertKnownUser(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx,
		`INSERT INTO known_users (id) VALUES ($1) ON CONFLICT (id) DO UPDATE SET deleted_at = NULL`, id)
	return err
}

func (q *Querier) MarkKnownUserDeleted(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, `UPDATE known_users SET deleted_at = NOW() WHERE id = $1`, id)
	return err
}

// ── Documents ──────────────────────────────────────────────────────────────────

func (q *Querier) CreateDocument(ctx context.Context, id, projectID uuid.UUID, name, contentType string, createdBy uuid.UUID) (*Document, error) {
	row := q.db.QueryRow(ctx,
		`INSERT INTO documents (id, project_id, name, content_type, created_by)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, project_id, name, content_type, current_version, status, created_by, created_at, updated_at, deleted_at`,
		id, projectID, name, contentType, createdBy)
	return scanDocument(row)
}

func (q *Querier) GetDocument(ctx context.Context, id uuid.UUID) (*Document, error) {
	row := q.db.QueryRow(ctx,
		`SELECT id, project_id, name, content_type, current_version, status, created_by, created_at, updated_at, deleted_at
		 FROM documents WHERE id = $1 AND deleted_at IS NULL AND status != 'deleted'`, id)
	return scanDocument(row)
}

func (q *Querier) GetDocumentInternal(ctx context.Context, id uuid.UUID) (*Document, error) {
	row := q.db.QueryRow(ctx,
		`SELECT id, project_id, name, content_type, current_version, status, created_by, created_at, updated_at, deleted_at
		 FROM documents WHERE id = $1`, id)
	return scanDocument(row)
}

func (q *Querier) ListDocumentsByProject(ctx context.Context, projectID uuid.UUID, cursor *time.Time, cursorID *uuid.UUID, limit int) ([]*Document, error) {
	var rows Rows
	var err error
	if cursor != nil && cursorID != nil {
		rows, err = q.db.Query(ctx,
			`SELECT id, project_id, name, content_type, current_version, status, created_by, created_at, updated_at, deleted_at
			 FROM documents
			 WHERE project_id = $1 AND deleted_at IS NULL AND status = 'active'
			   AND (created_at, id) < ($2, $3)
			 ORDER BY created_at DESC, id DESC LIMIT $4`,
			projectID, cursor, cursorID, limit)
	} else {
		rows, err = q.db.Query(ctx,
			`SELECT id, project_id, name, content_type, current_version, status, created_by, created_at, updated_at, deleted_at
			 FROM documents
			 WHERE project_id = $1 AND deleted_at IS NULL AND status = 'active'
			 ORDER BY created_at DESC, id DESC LIMIT $2`,
			projectID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocuments(rows)
}

func (q *Querier) UpdateDocumentName(ctx context.Context, id uuid.UUID, name string) (*Document, error) {
	row := q.db.QueryRow(ctx,
		`UPDATE documents SET name = $2, updated_at = NOW() WHERE id = $1
		 RETURNING id, project_id, name, content_type, current_version, status, created_by, created_at, updated_at, deleted_at`,
		id, name)
	return scanDocument(row)
}

func (q *Querier) ActivateDocument(ctx context.Context, id uuid.UUID, versionNumber int) (*Document, error) {
	row := q.db.QueryRow(ctx,
		`UPDATE documents SET status = 'active', current_version = $2, updated_at = NOW() WHERE id = $1
		 RETURNING id, project_id, name, content_type, current_version, status, created_by, created_at, updated_at, deleted_at`,
		id, versionNumber)
	return scanDocument(row)
}

func (q *Querier) SetCurrentVersion(ctx context.Context, id uuid.UUID, versionNumber int) (*Document, error) {
	row := q.db.QueryRow(ctx,
		`UPDATE documents SET current_version = $2, updated_at = NOW() WHERE id = $1
		 RETURNING id, project_id, name, content_type, current_version, status, created_by, created_at, updated_at, deleted_at`,
		id, versionNumber)
	return scanDocument(row)
}

func (q *Querier) SoftDeleteDocument(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx,
		`UPDATE documents SET status = 'deleted', deleted_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	return err
}

func (q *Querier) SoftDeleteDocumentsByProject(ctx context.Context, projectID uuid.UUID) ([]struct{ ID, ProjectID uuid.UUID }, error) {
	rows, err := q.db.Query(ctx,
		`UPDATE documents SET status = 'deleted', deleted_at = NOW(), updated_at = NOW()
		 WHERE project_id = $1 AND status != 'deleted'
		 RETURNING id, project_id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []struct{ ID, ProjectID uuid.UUID }
	for rows.Next() {
		var r struct{ ID, ProjectID uuid.UUID }
		if err := rows.Scan(&r.ID, &r.ProjectID); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (q *Querier) ListPendingDocumentsOlderThan(ctx context.Context, before time.Time) ([]*Document, error) {
	rows, err := q.db.Query(ctx,
		`SELECT id, project_id, name, content_type, current_version, status, created_by, created_at, updated_at, deleted_at
		 FROM documents WHERE status = 'pending' AND created_at < $1`, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocuments(rows)
}

func (q *Querier) ListSoftDeletedOlderThan(ctx context.Context, before time.Time, limit int) ([]*Document, error) {
	rows, err := q.db.Query(ctx,
		`SELECT id, project_id, name, content_type, current_version, status, created_by, created_at, updated_at, deleted_at
		 FROM documents WHERE status = 'deleted' AND deleted_at < $1 ORDER BY deleted_at LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocuments(rows)
}

func (q *Querier) HardDeleteDocument(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, `DELETE FROM documents WHERE id = $1`, id)
	return err
}

// ── Versions ───────────────────────────────────────────────────────────────────

func (q *Querier) CreateVersion(ctx context.Context, id, documentID uuid.UUID, versionNumber int, storageKey, contentType string, sizeBytes int64, uploadedBy uuid.UUID) (*DocumentVersion, error) {
	row := q.db.QueryRow(ctx,
		`INSERT INTO document_versions (id, document_id, version_number, storage_key, content_type, size_bytes, uploaded_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, document_id, version_number, storage_key, content_type, size_bytes, checksum_sha256, status, uploaded_by, uploaded_at, created_at`,
		id, documentID, versionNumber, storageKey, contentType, sizeBytes, uploadedBy)
	return scanVersion(row)
}

func (q *Querier) GetVersion(ctx context.Context, documentID uuid.UUID, versionNumber int) (*DocumentVersion, error) {
	row := q.db.QueryRow(ctx,
		`SELECT id, document_id, version_number, storage_key, content_type, size_bytes, checksum_sha256, status, uploaded_by, uploaded_at, created_at
		 FROM document_versions WHERE document_id = $1 AND version_number = $2`, documentID, versionNumber)
	return scanVersion(row)
}

func (q *Querier) GetCurrentVersion(ctx context.Context, documentID uuid.UUID) (*DocumentVersion, error) {
	row := q.db.QueryRow(ctx,
		`SELECT dv.id, dv.document_id, dv.version_number, dv.storage_key, dv.content_type, dv.size_bytes,
		        dv.checksum_sha256, dv.status, dv.uploaded_by, dv.uploaded_at, dv.created_at
		 FROM document_versions dv
		 JOIN documents d ON d.id = dv.document_id
		 WHERE dv.document_id = $1 AND dv.version_number = d.current_version`, documentID)
	return scanVersion(row)
}

func (q *Querier) ListVersionsByDocument(ctx context.Context, documentID uuid.UUID) ([]*DocumentVersion, error) {
	rows, err := q.db.Query(ctx,
		`SELECT id, document_id, version_number, storage_key, content_type, size_bytes, checksum_sha256, status, uploaded_by, uploaded_at, created_at
		 FROM document_versions WHERE document_id = $1 ORDER BY version_number DESC`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanVersions(rows)
}

func (q *Querier) GetMaxVersionNumber(ctx context.Context, documentID uuid.UUID) (int, error) {
	var maxNum int
	err := q.db.QueryRow(ctx,
		`SELECT COALESCE(MAX(version_number), 0) FROM document_versions WHERE document_id = $1`, documentID).Scan(&maxNum)
	return maxNum, err
}

func (q *Querier) MarkVersionUploaded(ctx context.Context, id uuid.UUID, checksum *string) (*DocumentVersion, error) {
	row := q.db.QueryRow(ctx,
		`UPDATE document_versions SET status = 'uploaded', uploaded_at = NOW(), checksum_sha256 = $2 WHERE id = $1
		 RETURNING id, document_id, version_number, storage_key, content_type, size_bytes, checksum_sha256, status, uploaded_by, uploaded_at, created_at`,
		id, checksum)
	return scanVersion(row)
}

func (q *Querier) MarkVersionFailed(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, `UPDATE document_versions SET status = 'failed' WHERE id = $1`, id)
	return err
}

func (q *Querier) ListPendingVersionsOlderThan(ctx context.Context, before time.Time) ([]*DocumentVersion, error) {
	rows, err := q.db.Query(ctx,
		`SELECT id, document_id, version_number, storage_key, content_type, size_bytes, checksum_sha256, status, uploaded_by, uploaded_at, created_at
		 FROM document_versions WHERE status = 'pending' AND created_at < $1`, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanVersions(rows)
}

func (q *Querier) ListFailedVersions(ctx context.Context, limit int) ([]*DocumentVersion, error) {
	rows, err := q.db.Query(ctx,
		`SELECT id, document_id, version_number, storage_key, content_type, size_bytes, checksum_sha256, status, uploaded_by, uploaded_at, created_at
		 FROM document_versions WHERE status = 'failed' ORDER BY created_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanVersions(rows)
}

func (q *Querier) DeleteVersion(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, `DELETE FROM document_versions WHERE id = $1`, id)
	return err
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
	var events []*OutboxEvent
	for rows.Next() {
		e := &OutboxEvent{}
		if err := rows.Scan(&e.ID, &e.AggregateID, &e.EventType, &e.Payload, &e.OccurredAt, &e.PublishedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (q *Querier) MarkEventPublished(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, `UPDATE outbox SET published_at = NOW() WHERE id = $1`, id)
	return err
}

// ── Idempotency ────────────────────────────────────────────────────────────────

func (q *Querier) WasEventProcessed(ctx context.Context, eventID string) (bool, error) {
	var exists bool
	err := q.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1)`, eventID).Scan(&exists)
	return exists, err
}

func (q *Querier) MarkEventProcessed(ctx context.Context, eventID string) error {
	_, err := q.db.Exec(ctx,
		`INSERT INTO processed_events (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, eventID)
	return err
}

// ── Scan helpers ───────────────────────────────────────────────────────────────

func scanDocument(row Row) (*Document, error) {
	d := &Document{}
	err := row.Scan(&d.ID, &d.ProjectID, &d.Name, &d.ContentType, &d.CurrentVersion,
		&d.Status, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func scanDocuments(rows Rows) ([]*Document, error) {
	var docs []*Document
	for rows.Next() {
		d := &Document{}
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.Name, &d.ContentType, &d.CurrentVersion,
			&d.Status, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

func scanVersion(row Row) (*DocumentVersion, error) {
	v := &DocumentVersion{}
	err := row.Scan(&v.ID, &v.DocumentID, &v.VersionNumber, &v.StorageKey, &v.ContentType,
		&v.SizeBytes, &v.ChecksumSHA256, &v.Status, &v.UploadedBy, &v.UploadedAt, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func scanVersions(rows Rows) ([]*DocumentVersion, error) {
	var vers []*DocumentVersion
	for rows.Next() {
		v := &DocumentVersion{}
		if err := rows.Scan(&v.ID, &v.DocumentID, &v.VersionNumber, &v.StorageKey, &v.ContentType,
			&v.SizeBytes, &v.ChecksumSHA256, &v.Status, &v.UploadedBy, &v.UploadedAt, &v.CreatedAt); err != nil {
			return nil, err
		}
		vers = append(vers, v)
	}
	return vers, rows.Err()
}
