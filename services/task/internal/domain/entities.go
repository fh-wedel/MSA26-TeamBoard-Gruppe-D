package domain

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
	Status      Status
	Priority    Priority
	AssigneeID  *uuid.UUID
	DueDate     *time.Time
	StartDate   *time.Time
	Labels      []string
	Position    string
	CreatedBy   uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time

	// Counts populated by GetTaskWithCounts
	CommentCount    int
	AttachmentCount int
}

type Comment struct {
	ID        uuid.UUID
	TaskID    uuid.UUID
	AuthorID  uuid.UUID
	Body      string
	Mentions  []uuid.UUID
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
	ChangeType ChangeType
	Diff       map[string]any
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
	// Status is the explicit semantic status set on the source board column. When
	// empty (legacy data), the status is derived from Name via DeriveStatus.
	Status string
}

type OutboxEvent struct {
	ID          uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	Payload     []byte
	OccurredAt  time.Time
	PublishedAt *time.Time
}

// ── Enums ─────────────────────────────────────────────────────────────────────

type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusBlocked    Status = "blocked"
	StatusDone       Status = "done"
	StatusArchived   Status = "archived"
)

func ValidStatus(s Status) bool {
	switch s {
	case StatusOpen, StatusInProgress, StatusBlocked, StatusDone, StatusArchived:
		return true
	}
	return false
}

type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityMedium   Priority = "medium"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

func ValidPriority(p Priority) bool {
	switch p {
	case PriorityLow, PriorityMedium, PriorityHigh, PriorityCritical:
		return true
	}
	return false
}

type ChangeType string

const (
	ChangeCreated           ChangeType = "created"
	ChangeUpdated           ChangeType = "updated"
	ChangeMoved             ChangeType = "moved"
	ChangeAssigned          ChangeType = "assigned"
	ChangeUnassigned        ChangeType = "unassigned"
	ChangeCommented         ChangeType = "commented"
	ChangeAttachmentAdded   ChangeType = "attachment_added"
	ChangeAttachmentRemoved ChangeType = "attachment_removed"
	ChangeDeleted           ChangeType = "deleted"
)

// ── Input types ───────────────────────────────────────────────────────────────

type CreateTaskInput struct {
	BoardID     uuid.UUID
	ColumnID    *uuid.UUID
	Title       string
	Description string
	Priority    Priority
	AssigneeID  *uuid.UUID
	DueDate     *time.Time
	StartDate   *time.Time
	Labels      []string
}

type TaskPatch struct {
	Title        *string
	Description  *string
	Priority     *Priority
	Status       *Status
	DueDate      *time.Time
	DueDateSet   bool // true when DueDate is explicitly present in the patch (even if nil)
	StartDate    *time.Time
	StartDateSet bool // true when StartDate is explicitly present in the patch (even if nil)
	Labels       *[]string
}

type TaskFilter struct {
	Status     *Status
	AssigneeID *uuid.UUID
	ColumnID   *uuid.UUID
	Label      *string
}

type Pagination struct {
	Limit  int
	Cursor *string
}

type PageCursor struct {
	Position string    `json:"position"`
	ID       uuid.UUID `json:"id"`
}
