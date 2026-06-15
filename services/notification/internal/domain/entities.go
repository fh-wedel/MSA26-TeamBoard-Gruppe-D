package domain

import (
	"time"

	"github.com/google/uuid"
)

// ── Notification ──────────────────────────────────────────────────────────────

type Notification struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Type          NotificationType
	Payload       map[string]any
	ProjectID     *uuid.UUID
	SourceEventID *string
	CreatedAt     time.Time
	ReadAt        *time.Time
}

type NotificationType string

const (
	TypeMention               NotificationType = "mention"
	TypeTaskAssigned          NotificationType = "task_assigned"
	TypeTaskUnassigned        NotificationType = "task_unassigned"
	TypeTaskDueSoon           NotificationType = "task_due_soon"
	TypeTaskDeleted           NotificationType = "task_deleted"
	TypeCommentOnMyTask       NotificationType = "comment_on_my_task"
	TypeDocumentShared        NotificationType = "document_shared"
	TypeProjectMemberAdded      NotificationType = "project_member_added"
	TypeProjectMemberRemoved    NotificationType = "project_member_removed"
	TypeProjectInvitationSent   NotificationType = "project_invitation_sent"
	TypeProjectInvitationAccepted NotificationType = "project_invitation_accepted"
	TypeProjectInvitationDeclined NotificationType = "project_invitation_declined"
)

// ── Channel ───────────────────────────────────────────────────────────────────

type Channel string

func UserChannel(userID uuid.UUID) Channel    { return Channel("user:" + userID.String()) }
func ProjectChannel(id uuid.UUID) Channel     { return Channel("project:" + id.String()) }
func BoardChannel(id uuid.UUID) Channel       { return Channel("board:" + id.String()) }
func TaskChannel(id uuid.UUID) Channel        { return Channel("task:" + id.String()) }

// ── Connection (in-memory, not persisted) ─────────────────────────────────────

type Connection struct {
	ID         string
	UserID     uuid.UUID
	InstanceID string
	Send       chan []byte
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

// ── Pagination ────────────────────────────────────────────────────────────────

type ListFilter struct {
	UnreadOnly bool
	Type       *NotificationType
	Limit      int
	Cursor     *string
}

type PageCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}
