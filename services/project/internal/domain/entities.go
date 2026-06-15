package domain

import (
	"time"

	"github.com/google/uuid"
)

// ── Core entities ─────────────────────────────────────────────────────────────

type Project struct {
	ID          uuid.UUID
	Name        string
	Description string
	OwnerID     uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

type ProjectMember struct {
	ProjectID uuid.UUID
	UserID    uuid.UUID
	Email     string // denormalized from known_users JOIN
	Role      Role
	InvitedBy *uuid.UUID
	JoinedAt  time.Time
}

type Board struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	Name      string
	Type      BoardType
	Position  int
	Config    map[string]any
	CreatedBy uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
	Columns   []BoardColumn
}

type BoardColumn struct {
	ID       uuid.UUID
	BoardID  uuid.UUID
	Name     string
	Position int
	WIPLimit *int
}

type KnownUser struct {
	ID        uuid.UUID
	Email     string
	CreatedAt time.Time
	DeletedAt *time.Time
}

// ── Enums ─────────────────────────────────────────────────────────────────────

type Role string

const (
	RoleViewer Role = "viewer"
	RoleEditor Role = "editor"
	RoleOwner  Role = "owner"
)

func ValidRole(r Role) bool {
	return r == RoleViewer || r == RoleEditor || r == RoleOwner
}

type BoardType string

const (
	BoardTypeKanban   BoardType = "kanban"
	BoardTypeScrum    BoardType = "scrum"
	BoardTypeCalendar BoardType = "calendar"
)

// ── Input types ───────────────────────────────────────────────────────────────

type ProjectPatch struct {
	Name        *string
	Description *string
}

type BoardInput struct {
	Name    string
	Type    BoardType
	Config  map[string]any
	Columns []BoardColumnInput
}

type BoardColumnInput struct {
	Name     string
	Position int
	WIPLimit *int
}

type BoardPatch struct {
	Name   *string
	Config map[string]any
}

// ── Invitations ───────────────────────────────────────────────────────────────

type InvitationStatus string

const (
	InvitationStatusPending  InvitationStatus = "pending"
	InvitationStatusAccepted InvitationStatus = "accepted"
	InvitationStatusDeclined InvitationStatus = "declined"
)

type Invitation struct {
	ID           uuid.UUID
	ProjectID    uuid.UUID
	InviteeEmail string
	Role         Role
	Token        string
	InvitedBy    uuid.UUID
	Status       InvitationStatus
	CreatedAt    time.Time
	ExpiresAt    time.Time
	RespondedAt  *time.Time
}

func (i *Invitation) IsExpired() bool {
	return time.Now().After(i.ExpiresAt)
}

// ── Outbox ────────────────────────────────────────────────────────────────────

type OutboxEvent struct {
	ID          uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	Payload     []byte
	OccurredAt  time.Time
	PublishedAt *time.Time
}

// ── Permission types ──────────────────────────────────────────────────────────

type PermissionSet struct {
	Role          Role     `json:"role"`
	Permissions   []string `json:"permissions"`
	ProjectExists bool     `json:"project_exists"`
	IsMember      bool     `json:"is_member"`
}
