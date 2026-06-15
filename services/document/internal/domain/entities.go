package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ── Core entities ─────────────────────────────────────────────────────────────

type Document struct {
	ID             uuid.UUID
	ProjectID      uuid.UUID
	Name           string
	ContentType    string
	CurrentVersion *int
	Status         DocStatus
	CreatedBy      uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time

	// Populated from the current version when listing/getting.
	SizeBytes int64
}

type DocumentVersion struct {
	ID             uuid.UUID
	DocumentID     uuid.UUID
	VersionNumber  int
	StorageKey     string
	ContentType    string
	SizeBytes      int64
	ChecksumSHA256 *string
	Status         VersionStatus
	UploadedBy     uuid.UUID
	UploadedAt     *time.Time
	CreatedAt      time.Time
}

// ── Status enums ──────────────────────────────────────────────────────────────

type DocStatus string

const (
	DocStatusPending DocStatus = "pending"
	DocStatusActive  DocStatus = "active"
	DocStatusDeleted DocStatus = "deleted"
)

type VersionStatus string

const (
	VersionStatusPending  VersionStatus = "pending"
	VersionStatusUploaded VersionStatus = "uploaded"
	VersionStatusFailed   VersionStatus = "failed"
)

// ── Result types with pre-signed URLs ─────────────────────────────────────────

type UploadInitiation struct {
	Document  *Document
	Version   *DocumentVersion
	UploadURL string
	ExpiresAt time.Time
}

type DownloadInfo struct {
	Document    *Document
	Version     *DocumentVersion
	DownloadURL string
	ExpiresAt   time.Time
}

// ── Input types ───────────────────────────────────────────────────────────────

type InitiateUploadInput struct {
	ProjectID   uuid.UUID
	Name        string
	ContentType string
	SizeBytes   int64
}

type NewVersionInput struct {
	ContentType string
	SizeBytes   int64
}

// ── Pagination ────────────────────────────────────────────────────────────────

type Pagination struct {
	Limit  int
	Cursor *string
}

type PageCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

// ── Outbox event ──────────────────────────────────────────────────────────────

type OutboxEvent struct {
	ID          uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	Payload     []byte
	OccurredAt  time.Time
	PublishedAt *time.Time
}

// ── Storage-key builder ───────────────────────────────────────────────────────

var nonAlphanumRE = regexp.MustCompile(`[^a-z0-9.\-]+`)

// BuildStorageKey generates a deterministic, safe S3/MinIO object key.
func BuildStorageKey(projectID, documentID uuid.UUID, version int, name string) string {
	safe := nonAlphanumRE.ReplaceAllString(strings.ToLower(name), "-")
	if len(safe) > 100 {
		safe = safe[:100]
	}
	return fmt.Sprintf("projects/%s/documents/%s/v%d/%s", projectID, documentID, version, safe)
}

// ── Content-type allowlist ────────────────────────────────────────────────────

// DefaultAllowedContentTypes is used when no allowlist is configured.
var DefaultAllowedContentTypes = []string{
	"application/pdf",
	"image/png",
	"image/jpeg",
	"image/gif",
	"application/zip",
	"text/plain",
	"text/markdown",
	"text/x-markdown",
	"application/octet-stream",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation",
}

// IsContentTypeAllowed returns true when ct appears in the allowlist.
func IsContentTypeAllowed(ct string, allowlist []string) bool {
	for _, allowed := range allowlist {
		if strings.EqualFold(ct, allowed) {
			return true
		}
	}
	return false
}
