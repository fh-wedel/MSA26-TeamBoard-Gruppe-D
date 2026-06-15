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
	// Projects
	CreateProject(ctx context.Context, id, ownerID uuid.UUID, name, description string) (*Project, error)
	GetProject(ctx context.Context, id uuid.UUID) (*Project, error)
	ListProjectsForUser(ctx context.Context, userID uuid.UUID) ([]*Project, error)
	UpdateProject(ctx context.Context, id uuid.UUID, name, description *string) (*Project, error)
	SoftDeleteProject(ctx context.Context, id uuid.UUID) error

	// Members
	AddMember(ctx context.Context, projectID, userID uuid.UUID, role string, invitedBy *uuid.UUID) (*ProjectMember, error)
	GetMember(ctx context.Context, projectID, userID uuid.UUID) (*ProjectMember, error)
	ListMembers(ctx context.Context, projectID uuid.UUID) ([]*ProjectMemberWithEmail, error)
	UpdateMemberRole(ctx context.Context, projectID, userID uuid.UUID, role string) (*ProjectMember, error)
	RemoveMember(ctx context.Context, projectID, userID uuid.UUID) error
	CountOwners(ctx context.Context, projectID uuid.UUID) (int64, error)
	GetMemberRole(ctx context.Context, projectID, userID uuid.UUID) (string, error)
	RemoveAllMembershipsOfUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)

	// Boards
	CreateBoard(ctx context.Context, id, projectID uuid.UUID, name, bType string, position int, config []byte, createdBy uuid.UUID) (*Board, error)
	GetBoard(ctx context.Context, id uuid.UUID) (*Board, error)
	ListBoardsByProject(ctx context.Context, projectID uuid.UUID) ([]*Board, error)
	UpdateBoard(ctx context.Context, id uuid.UUID, name *string, config []byte) (*Board, error)
	SoftDeleteBoard(ctx context.Context, id uuid.UUID) error
	GetMaxBoardPosition(ctx context.Context, projectID uuid.UUID) (int, error)

	// Columns
	CreateColumn(ctx context.Context, id, boardID uuid.UUID, name string, position int, wipLimit *int, status string) (*BoardColumn, error)
	ListColumnsByBoard(ctx context.Context, boardID uuid.UUID) ([]BoardColumn, error)

	// Known users
	UpsertKnownUser(ctx context.Context, id uuid.UUID, email string, createdAt time.Time) error
	GetKnownUserByEmail(ctx context.Context, email string) (*KnownUser, error)
	GetKnownUserByID(ctx context.Context, id uuid.UUID) (*KnownUser, error)
	MarkKnownUserDeleted(ctx context.Context, id uuid.UUID) error

	// Outbox
	InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error
	GetUnpublishedEvents(ctx context.Context, limit int32) ([]*OutboxEvent, error)
	MarkEventPublished(ctx context.Context, id uuid.UUID) error

	// Processed events
	WasEventProcessed(ctx context.Context, eventID string) (bool, error)
	MarkEventProcessed(ctx context.Context, eventID string) error
}

type queries struct{ db DBTX }

func New(db DBTX) Querier { return &queries{db: db} }
