package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TaskService is the application interface for all task use-cases.
type TaskService interface {
	// Tasks
	CreateTask(ctx context.Context, requester uuid.UUID, input CreateTaskInput) (*Task, error)
	GetTask(ctx context.Context, taskID, requester uuid.UUID) (*Task, error)
	ListTasks(ctx context.Context, boardID, requester uuid.UUID, filter TaskFilter, page Pagination) ([]*Task, *PageCursor, error)
	UpdateTask(ctx context.Context, taskID, requester uuid.UUID, patch TaskPatch) (*Task, error)
	MoveTask(ctx context.Context, taskID, requester uuid.UUID, columnID uuid.UUID, beforeID, afterID *uuid.UUID) (*Task, error)
	AssignTask(ctx context.Context, taskID, requester uuid.UUID, assigneeID *uuid.UUID) (*Task, error)
	DeleteTask(ctx context.Context, taskID, requester uuid.UUID) error

	// Comments
	CreateComment(ctx context.Context, taskID, requester uuid.UUID, body string) (*Comment, error)
	ListComments(ctx context.Context, taskID, requester uuid.UUID) ([]*Comment, error)
	UpdateComment(ctx context.Context, commentID, requester uuid.UUID, body string) (*Comment, error)
	DeleteComment(ctx context.Context, commentID, requester uuid.UUID) error

	// Attachments
	AddAttachment(ctx context.Context, taskID, requester, documentID uuid.UUID) (*Attachment, error)
	ListAttachments(ctx context.Context, taskID, requester uuid.UUID) ([]*Attachment, error)
	RemoveAttachment(ctx context.Context, attachmentID, requester uuid.UUID) error

	// History
	GetTaskHistory(ctx context.Context, taskID, requester uuid.UUID) ([]*HistoryEntry, error)
}

// Repository is the persistence interface consumed by the domain.
type Repository interface {
	WithTransaction(ctx context.Context, fn func(context.Context, Repository) error) error

	// Tasks
	CreateTask(ctx context.Context, id, boardID, projectID uuid.UUID, columnID *uuid.UUID, title, description string, status Status, priority Priority, assigneeID *uuid.UUID, dueDate, startDate *time.Time, labels []string, position string, createdBy uuid.UUID) (*Task, error)
	GetTask(ctx context.Context, id uuid.UUID) (*Task, error)
	GetTaskWithCounts(ctx context.Context, id uuid.UUID) (*Task, error)
	ListTasksByBoard(ctx context.Context, boardID uuid.UUID, filter TaskFilter, cursor *string, limit int) ([]*Task, error)
	GetLastPositionInColumn(ctx context.Context, boardID uuid.UUID, columnID *uuid.UUID) (string, error)
	GetPositionForRefs(ctx context.Context, beforeID, afterID *uuid.UUID) (beforePos, afterPos string, err error)
	UpdateTask(ctx context.Context, id uuid.UUID, patch TaskPatch) (*Task, error)
	MoveTask(ctx context.Context, id, columnID uuid.UUID, position string, status Status) (*Task, error)
	AssignTask(ctx context.Context, id uuid.UUID, assigneeID *uuid.UUID) (*Task, error)
	SoftDeleteTask(ctx context.Context, id uuid.UUID) error
	SoftDeleteTasksByProject(ctx context.Context, projectID uuid.UUID) ([]uuid.UUID, error)
	SoftDeleteTasksByBoard(ctx context.Context, boardID uuid.UUID) ([]uuid.UUID, error)
	ClearAssigneeForUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	ClearAssigneeForUserInProject(ctx context.Context, userID, projectID uuid.UUID) ([]uuid.UUID, error)
	NullifyColumnReferences(ctx context.Context, columnID uuid.UUID) error

	// Comments
	CreateComment(ctx context.Context, id, taskID, authorID uuid.UUID, body string) (*Comment, error)
	GetComment(ctx context.Context, id uuid.UUID) (*Comment, error)
	ListCommentsByTask(ctx context.Context, taskID uuid.UUID) ([]*Comment, error)
	UpdateComment(ctx context.Context, id uuid.UUID, body string) (*Comment, error)
	SoftDeleteComment(ctx context.Context, id uuid.UUID) error
	ArchiveCommentBody(ctx context.Context, historyID, commentID uuid.UUID, body string) error
	AddMention(ctx context.Context, commentID, userID uuid.UUID) error
	ListMentionsByComment(ctx context.Context, commentID uuid.UUID) ([]uuid.UUID, error)
	ClearMentions(ctx context.Context, commentID uuid.UUID) error

	// Attachments
	CreateAttachment(ctx context.Context, id, taskID, documentID, addedBy uuid.UUID) (*Attachment, error)
	GetAttachment(ctx context.Context, id uuid.UUID) (*Attachment, error)
	ListAttachmentsByTask(ctx context.Context, taskID uuid.UUID) ([]*Attachment, error)
	DeleteAttachment(ctx context.Context, id uuid.UUID) error
	DeleteAttachmentsByDocument(ctx context.Context, documentID uuid.UUID) ([]struct{ ID, TaskID uuid.UUID }, error)

	// History
	CreateHistoryEntry(ctx context.Context, id, taskID, actorID uuid.UUID, changeType ChangeType, diff map[string]any) error
	ListTaskHistory(ctx context.Context, taskID uuid.UUID, limit int) ([]*HistoryEntry, error)

	// Known boards / columns / users
	UpsertKnownBoard(ctx context.Context, id, projectID uuid.UUID, name, bType string) error
	GetKnownBoard(ctx context.Context, id uuid.UUID) (*KnownBoard, error)
	MarkBoardDeleted(ctx context.Context, id uuid.UUID) error
	UpsertKnownColumn(ctx context.Context, id, boardID uuid.UUID, name string, position int, status string) error
	GetKnownColumn(ctx context.Context, id uuid.UUID) (*KnownColumn, error)
	ColumnBelongsToBoard(ctx context.Context, columnID, boardID uuid.UUID) (bool, error)
	DeleteKnownColumn(ctx context.Context, id uuid.UUID) error
	UpsertKnownUser(ctx context.Context, id uuid.UUID, email string) error
	KnownUserExists(ctx context.Context, id uuid.UUID) (bool, error)
	MarkKnownUserDeleted(ctx context.Context, id uuid.UUID) error

	// Outbox
	InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error
	GetUnpublishedEvents(ctx context.Context, limit int32) ([]*OutboxEvent, error)
	MarkEventPublished(ctx context.Context, id uuid.UUID) error

	// Idempotency
	WasEventProcessed(ctx context.Context, eventID string) (bool, error)
	MarkEventProcessed(ctx context.Context, eventID string) error
}

// ProjectClient fetches permission sets from the Project Service.
type ProjectClient interface {
	GetPermissions(ctx context.Context, projectID, userID uuid.UUID) (*PermissionSet, error)
}

// DocumentClient validates documents against the Document Service.
type DocumentClient interface {
	GetDocumentInfo(ctx context.Context, documentID uuid.UUID) (*DocumentInfo, error)
}

// PermissionSet is the cached response from the Project Service.
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

// DocumentInfo is the minimal document metadata from the Document Service.
type DocumentInfo struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
}
