package db

import (
	"time"

	"github.com/google/uuid"
)

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
	Role      string
	InvitedBy *uuid.UUID
	JoinedAt  time.Time
}

type ProjectMemberWithEmail struct {
	ProjectID uuid.UUID
	UserID    uuid.UUID
	Role      string
	InvitedBy *uuid.UUID
	JoinedAt  time.Time
	Email     *string
}

type Board struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	Name      string
	Type      string
	Position  int
	Config    []byte
	CreatedBy uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

type BoardColumn struct {
	ID        uuid.UUID
	BoardID   uuid.UUID
	Name      string
	Position  int
	WIPLimit  *int
	CreatedAt time.Time
}

type KnownUser struct {
	ID        uuid.UUID
	Email     string
	CreatedAt time.Time
	DeletedAt *time.Time
}

type OutboxEvent struct {
	ID          uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	Payload     []byte
	OccurredAt  time.Time
	PublishedAt *time.Time
}
