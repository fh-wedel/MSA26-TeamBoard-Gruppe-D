package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// DocumentService is the application interface for all document use-cases.
type DocumentService interface {
	// Upload (two-phase)
	InitiateUpload(ctx context.Context, requester uuid.UUID, input InitiateUploadInput) (*UploadInitiation, error)
	InitiateNewVersion(ctx context.Context, documentID, requester uuid.UUID, input NewVersionInput) (*UploadInitiation, error)
	ConfirmUpload(ctx context.Context, documentID, requester uuid.UUID, versionNumber int) (*Document, error)

	// Read
	GetDocument(ctx context.Context, documentID, requester uuid.UUID) (*Document, error)
	GetDownloadURL(ctx context.Context, documentID, requester uuid.UUID, version *int) (*DownloadInfo, error)
	ListVersions(ctx context.Context, documentID, requester uuid.UUID) ([]*DocumentVersion, error)
	ListDocuments(ctx context.Context, projectID, requester uuid.UUID, page Pagination) ([]*Document, *PageCursor, error)

	// Modify
	RenameDocument(ctx context.Context, documentID, requester uuid.UUID, newName string) (*Document, error)
	DeleteDocument(ctx context.Context, documentID, requester uuid.UUID) error
	RestoreVersion(ctx context.Context, documentID, requester uuid.UUID, versionNumber int) (*Document, error)

	// Internal (service-to-service)
	GetDocumentInfo(ctx context.Context, documentID uuid.UUID) (*DocumentInfo, error)
}

// DocumentInfo is the minimal metadata returned to other services. The JSON
// tags are required: consumers (e.g. the Task service's document client) decode
// this over the internal API into snake_case-tagged structs, and Go's field
// matcher will not map "ProjectID" onto `json:"project_id"`.
type DocumentInfo struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	Name      string    `json:"name"`
	Exists    bool      `json:"exists"`
	Active    bool      `json:"active"`
}

// Repository is the persistence interface consumed by the domain.
type Repository interface {
	WithTransaction(ctx context.Context, fn func(context.Context, Repository) error) error

	// Documents
	CreateDocument(ctx context.Context, id, projectID uuid.UUID, name, contentType string, createdBy uuid.UUID) (*Document, error)
	GetDocument(ctx context.Context, id uuid.UUID) (*Document, error)
	GetDocumentInternal(ctx context.Context, id uuid.UUID) (*Document, error)
	ListDocumentsByProject(ctx context.Context, projectID uuid.UUID, cursor *time.Time, cursorID *uuid.UUID, limit int) ([]*Document, error)
	UpdateDocumentName(ctx context.Context, id uuid.UUID, name string) (*Document, error)
	ActivateDocument(ctx context.Context, id uuid.UUID, versionNumber int) (*Document, error)
	SetCurrentVersion(ctx context.Context, id uuid.UUID, versionNumber int) (*Document, error)
	SoftDeleteDocument(ctx context.Context, id uuid.UUID) error
	SoftDeleteDocumentsByProject(ctx context.Context, projectID uuid.UUID) ([]struct{ ID, ProjectID uuid.UUID }, error)
	ListPendingDocumentsOlderThan(ctx context.Context, before time.Time) ([]*Document, error)
	ListSoftDeletedOlderThan(ctx context.Context, before time.Time, limit int) ([]*Document, error)
	HardDeleteDocument(ctx context.Context, id uuid.UUID) error

	// Versions
	CreateVersion(ctx context.Context, id, documentID uuid.UUID, versionNumber int, storageKey, contentType string, sizeBytes int64, uploadedBy uuid.UUID) (*DocumentVersion, error)
	GetVersion(ctx context.Context, documentID uuid.UUID, versionNumber int) (*DocumentVersion, error)
	GetCurrentVersion(ctx context.Context, documentID uuid.UUID) (*DocumentVersion, error)
	ListVersionsByDocument(ctx context.Context, documentID uuid.UUID) ([]*DocumentVersion, error)
	GetMaxVersionNumber(ctx context.Context, documentID uuid.UUID) (int, error)
	MarkVersionUploaded(ctx context.Context, id uuid.UUID, checksum *string) (*DocumentVersion, error)
	MarkVersionFailed(ctx context.Context, id uuid.UUID) error
	ListPendingVersionsOlderThan(ctx context.Context, before time.Time) ([]*DocumentVersion, error)
	ListFailedVersions(ctx context.Context, limit int) ([]*DocumentVersion, error)
	DeleteVersion(ctx context.Context, id uuid.UUID) error

	// Known entities
	UpsertKnownProject(ctx context.Context, id uuid.UUID) error
	KnownProjectExists(ctx context.Context, id uuid.UUID) (bool, error)
	MarkKnownProjectDeleted(ctx context.Context, id uuid.UUID) error
	UpsertKnownUser(ctx context.Context, id uuid.UUID) error
	MarkKnownUserDeleted(ctx context.Context, id uuid.UUID) error

	// Outbox
	InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error
	GetUnpublishedEvents(ctx context.Context, limit int32) ([]*OutboxEvent, error)
	MarkEventPublished(ctx context.Context, id uuid.UUID) error

	// Idempotency
	WasEventProcessed(ctx context.Context, eventID string) (bool, error)
	MarkEventProcessed(ctx context.Context, eventID string) error
}

// ProjectClient checks permissions against the Project Service.
type ProjectClient interface {
	GetPermissions(ctx context.Context, projectID, userID uuid.UUID) (*PermissionSet, error)
}

// PermissionSet is the response from the Project Service.
type PermissionSet struct {
	Role          string   `json:"role"`
	Permissions   []string `json:"permissions"`
	ProjectExists bool     `json:"project_exists"`
	IsMember      bool     `json:"is_member"`
}

func (ps *PermissionSet) Has(perm string) bool {
	for _, p := range ps.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}
