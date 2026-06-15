package db

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX is satisfied by *pgxpool.Pool and pgx.Tx.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Querier interface {
	// Tasks
	CreateTask(ctx context.Context, id, boardID, projectID uuid.UUID, columnID *uuid.UUID, title, description, status, priority string, assigneeID *uuid.UUID, dueDate *time.Time, labels []string, position string, createdBy uuid.UUID) (*Task, error)
	GetTask(ctx context.Context, id uuid.UUID) (*Task, error)
	GetTaskWithCounts(ctx context.Context, id uuid.UUID) (*TaskWithCounts, error)
	ListTasksByBoard(ctx context.Context, boardID uuid.UUID, statusFilter *string, assigneeFilter *uuid.UUID, columnFilter *uuid.UUID, labelFilter *string, cursorPos *string, limit int) ([]*Task, error)
	GetLastPositionInColumn(ctx context.Context, boardID uuid.UUID, columnID *uuid.UUID) (string, error)
	GetPositionsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error)
	UpdateTask(ctx context.Context, id uuid.UUID, title, description, priority *string, dueDateSet bool, dueDate *time.Time, labels []string) (*Task, error)
	MoveTask(ctx context.Context, id, columnID uuid.UUID, position, status string) (*Task, error)
	AssignTask(ctx context.Context, id uuid.UUID, assigneeID *uuid.UUID) (*Task, error)
	SoftDeleteTask(ctx context.Context, id uuid.UUID) error
	SoftDeleteTasksByProject(ctx context.Context, projectID uuid.UUID) ([]struct{ ID, ProjectID uuid.UUID }, error)
	SoftDeleteTasksByBoard(ctx context.Context, boardID uuid.UUID) ([]struct{ ID, ProjectID uuid.UUID }, error)
	ClearAssigneeForUser(ctx context.Context, userID uuid.UUID) ([]struct{ ID, ProjectID uuid.UUID }, error)
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
	CreateHistoryEntry(ctx context.Context, id, taskID, actorID uuid.UUID, changeType string, diff []byte) error
	ListTaskHistory(ctx context.Context, taskID uuid.UUID, limit int) ([]*HistoryEntry, error)

	// Known boards / columns / users
	UpsertKnownBoard(ctx context.Context, id, projectID uuid.UUID, name, bType string) error
	GetKnownBoard(ctx context.Context, id uuid.UUID) (*KnownBoard, error)
	MarkBoardDeleted(ctx context.Context, id uuid.UUID) error
	UpsertKnownColumn(ctx context.Context, id, boardID uuid.UUID, name string, position int) error
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

type queries struct{ db DBTX }

func New(db DBTX) Querier { return &queries{db: db} }
