package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository handles persistent notifications.
type Repository interface {
	CreateNotification(ctx context.Context, n *Notification) (*Notification, error)
	GetNotification(ctx context.Context, id, userID uuid.UUID) (*Notification, error)
	ListNotifications(ctx context.Context, userID uuid.UUID, filter ListFilter) ([]*Notification, error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
	MarkRead(ctx context.Context, id, userID uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
	DeleteNotification(ctx context.Context, id, userID uuid.UUID) error
	DeleteNotificationsByProject(ctx context.Context, projectID uuid.UUID) error
	DeleteOldNotifications(ctx context.Context, before time.Time) (int64, error)

	// Outbox
	InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error
	GetUnpublishedEvents(ctx context.Context, limit int32) ([]*OutboxEvent, error)
	MarkEventPublished(ctx context.Context, id uuid.UUID) error

	// Idempotency
	WasEventProcessed(ctx context.Context, eventID string) (bool, error)
	MarkEventProcessed(ctx context.Context, eventID string) error
}

// NotificationService is the application-level interface for notification CRUD.
type NotificationService interface {
	ListNotifications(ctx context.Context, userID uuid.UUID, filter ListFilter) ([]*Notification, *PageCursor, error)
	GetUnreadCount(ctx context.Context, userID uuid.UUID) (int, error)
	MarkRead(ctx context.Context, id, userID uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
	DeleteNotification(ctx context.Context, id, userID uuid.UUID) error
}

// ProjectClient checks permissions from the Project Service.
type ProjectClient interface {
	GetPermissions(ctx context.Context, projectID, userID uuid.UUID) (*PermissionSet, error)
}

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
