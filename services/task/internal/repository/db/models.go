package db

import (
	"time"

	"github.com/google/uuid"
)

type Task struct {
	ID          uuid.UUID
	BoardID     uuid.UUID
	ProjectID   uuid.UUID
	ColumnID    *uuid.UUID
	Title       string
	Description string
	Status      string
	Priority    string
	AssigneeID  *uuid.UUID
	DueDate     *time.Time
	Labels      []string
	Position    string
	CreatedBy   uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

type TaskWithCounts struct {
	Task
	CommentCount    int
	AttachmentCount int
}

type Comment struct {
	ID        uuid.UUID
	TaskID    uuid.UUID
	AuthorID  uuid.UUID
	Body      string
	EditedAt  *time.Time
	CreatedAt time.Time
	DeletedAt *time.Time
}

type Attachment struct {
	ID         uuid.UUID
	TaskID     uuid.UUID
	DocumentID uuid.UUID
	AddedBy    uuid.UUID
	AddedAt    time.Time
}

type HistoryEntry struct {
	ID         uuid.UUID
	TaskID     uuid.UUID
	ActorID    uuid.UUID
	ChangeType string
	Diff       []byte
	OccurredAt time.Time
}

type KnownBoard struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	Name      string
	Type      string
	DeletedAt *time.Time
}

type KnownColumn struct {
	ID       uuid.UUID
	BoardID  uuid.UUID
	Name     string
	Position int
}

type OutboxEvent struct {
	ID          uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	Payload     []byte
	OccurredAt  time.Time
	PublishedAt *time.Time
}
