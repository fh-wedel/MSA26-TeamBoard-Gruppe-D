package db

import (
	"time"

	"github.com/google/uuid"
)

type Document struct {
	ID             uuid.UUID
	ProjectID      uuid.UUID
	Name           string
	ContentType    string
	CurrentVersion *int
	Status         string
	CreatedBy      uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}

type DocumentVersion struct {
	ID             uuid.UUID
	DocumentID     uuid.UUID
	VersionNumber  int
	StorageKey     string
	ContentType    string
	SizeBytes      int64
	ChecksumSHA256 *string
	Status         string
	UploadedBy     uuid.UUID
	UploadedAt     *time.Time
	CreatedAt      time.Time
}

type OutboxEvent struct {
	ID          uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	Payload     []byte
	OccurredAt  time.Time
	PublishedAt *time.Time
}
