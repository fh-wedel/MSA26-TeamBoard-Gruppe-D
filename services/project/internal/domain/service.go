package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ProjectService defines all use-cases for the project service.
type ProjectService interface {
	// Projects
	CreateProject(ctx context.Context, ownerID uuid.UUID, name, description string) (*Project, error)
	GetProject(ctx context.Context, id, requester uuid.UUID) (*Project, error)
	ListProjectsForUser(ctx context.Context, userID uuid.UUID) ([]*Project, error)
	UpdateProject(ctx context.Context, id, requester uuid.UUID, patch ProjectPatch) (*Project, error)
	DeleteProject(ctx context.Context, id, requester uuid.UUID) error

	// Members
	AddMember(ctx context.Context, projectID, requester uuid.UUID, userID uuid.UUID, email string, role Role) (*ProjectMember, error)
	UpdateMemberRole(ctx context.Context, projectID, requester, userID uuid.UUID, newRole Role) (*ProjectMember, error)
	RemoveMember(ctx context.Context, projectID, requester, userID uuid.UUID) error
	ListMembers(ctx context.Context, projectID, requester uuid.UUID) ([]*ProjectMember, error)

	// Boards
	CreateBoard(ctx context.Context, projectID, requester uuid.UUID, input BoardInput) (*Board, error)
	GetBoard(ctx context.Context, boardID, requester uuid.UUID) (*Board, error)
	ListBoards(ctx context.Context, projectID, requester uuid.UUID) ([]*Board, error)
	UpdateBoard(ctx context.Context, boardID, requester uuid.UUID, patch BoardPatch) (*Board, error)
	DeleteBoard(ctx context.Context, boardID, requester uuid.UUID) error

	// Columns
	ListColumns(ctx context.Context, boardID, requester uuid.UUID) ([]BoardColumn, error)
	CreateColumn(ctx context.Context, boardID, requester uuid.UUID, input BoardColumnInput) (*BoardColumn, error)

	// Internal permission check (UC-9)
	GetPermissions(ctx context.Context, projectID, userID uuid.UUID) (*PermissionSet, error)

	// Invitations
	CreateInvitation(ctx context.Context, projectID, requester uuid.UUID, email string, role Role) (*Invitation, error)
	GetInvitationByToken(ctx context.Context, token string) (*Invitation, error)
	AcceptInvitation(ctx context.Context, token string, acceptorID uuid.UUID, acceptorEmail string) (*ProjectMember, error)
	DeclineInvitation(ctx context.Context, token string, declinerID uuid.UUID, declinerEmail string) error
	ListInvitations(ctx context.Context, projectID, requester uuid.UUID) ([]*Invitation, error)
}

// Repository is the persistence interface consumed by the domain layer.
type Repository interface {
	// Projects
	CreateProject(ctx context.Context, id, ownerID uuid.UUID, name, description string) (*Project, error)
	GetProject(ctx context.Context, id uuid.UUID) (*Project, error)
	ListProjectsForUser(ctx context.Context, userID uuid.UUID) ([]*Project, error)
	UpdateProject(ctx context.Context, id uuid.UUID, name, description *string) (*Project, error)
	SoftDeleteProject(ctx context.Context, id uuid.UUID) error

	// Members
	AddMember(ctx context.Context, projectID, userID uuid.UUID, role Role, invitedBy *uuid.UUID) (*ProjectMember, error)
	GetMember(ctx context.Context, projectID, userID uuid.UUID) (*ProjectMember, error)
	ListMembers(ctx context.Context, projectID uuid.UUID) ([]*ProjectMember, error)
	UpdateMemberRole(ctx context.Context, projectID, userID uuid.UUID, role Role) (*ProjectMember, error)
	RemoveMember(ctx context.Context, projectID, userID uuid.UUID) error
	CountOwners(ctx context.Context, projectID uuid.UUID) (int64, error)
	GetMemberRole(ctx context.Context, projectID, userID uuid.UUID) (Role, error)
	RemoveAllMembershipsOfUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)

	// Boards
	CreateBoard(ctx context.Context, id, projectID uuid.UUID, name string, bType BoardType, position int, config []byte, createdBy uuid.UUID) (*Board, error)
	GetBoard(ctx context.Context, id uuid.UUID) (*Board, error)
	ListBoardsByProject(ctx context.Context, projectID uuid.UUID) ([]*Board, error)
	UpdateBoard(ctx context.Context, id uuid.UUID, name *string, config []byte) (*Board, error)
	SoftDeleteBoard(ctx context.Context, id uuid.UUID) error
	GetMaxBoardPosition(ctx context.Context, projectID uuid.UUID) (int, error)

	// Columns
	CreateColumn(ctx context.Context, id, boardID uuid.UUID, name string, position int, wipLimit *int) (*BoardColumn, error)
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

	// Processed events (idempotency)
	WasEventProcessed(ctx context.Context, eventID string) (bool, error)
	MarkEventProcessed(ctx context.Context, eventID string) error

	// Invitations
	CreateInvitation(ctx context.Context, id uuid.UUID, projectID uuid.UUID, email, role, token string, invitedBy uuid.UUID) (*Invitation, error)
	GetInvitationByToken(ctx context.Context, token string) (*Invitation, error)
	GetPendingInvitationByEmailAndProject(ctx context.Context, email string, projectID uuid.UUID) (*Invitation, error)
	GetInvitationsByProject(ctx context.Context, projectID uuid.UUID) ([]*Invitation, error)
	UpdateInvitationStatus(ctx context.Context, id uuid.UUID, status InvitationStatus) error

	// Transaction support
	WithTransaction(ctx context.Context, fn func(context.Context, Repository) error) error
}

// PermissionCache caches resolved permission sets per (project, user).
type PermissionCache interface {
	Get(ctx context.Context, projectID, userID uuid.UUID) (*PermissionSet, error)
	Set(ctx context.Context, projectID, userID uuid.UUID, perms *PermissionSet) error
	Delete(ctx context.Context, projectID, userID uuid.UUID) error
}
