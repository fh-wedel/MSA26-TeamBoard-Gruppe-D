package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ── Envelope ─────────────────────────────────────────────────────────────────

type Actor struct {
	UserID string `json:"user_id"`
	Type   string `json:"type"`
}

type Envelope struct {
	EventID       string         `json:"event_id"`
	EventType     string         `json:"event_type"`
	OccurredAt    time.Time      `json:"occurred_at"`
	TraceID       string         `json:"trace_id"`
	Producer      string         `json:"producer"`
	AggregateType string         `json:"aggregate_type"`
	AggregateID   uuid.UUID      `json:"aggregate_id"`
	Actor         Actor          `json:"actor"`
	Payload       map[string]any `json:"payload"`
}

// ── Input / filter types ──────────────────────────────────────────────────────

type CreateWebhookInput struct {
	ProjectID   uuid.UUID
	TargetURL   string
	Description string
	EventFilter []string
}

type WebhookPatch struct {
	TargetURL   *string
	Description *string
	EventFilter *[]string
	Active      *bool
}

type DeliveryFilter struct {
	Status    *DeliveryStatus
	EventType *string
	Limit     int
	Cursor    *string // base64-encoded PageCursor
}

type Cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

// ── Service interfaces ────────────────────────────────────────────────────────

type WebhookService interface {
	CreateWebhook(ctx context.Context, requester uuid.UUID, input CreateWebhookInput) (*Webhook, string, error)
	GetWebhook(ctx context.Context, webhookID, requester uuid.UUID) (*Webhook, error)
	ListWebhooks(ctx context.Context, projectID, requester uuid.UUID) ([]*Webhook, error)
	UpdateWebhook(ctx context.Context, webhookID, requester uuid.UUID, patch WebhookPatch) (*Webhook, error)
	RotateSecret(ctx context.Context, webhookID, requester uuid.UUID) (string, error)
	EnableWebhook(ctx context.Context, webhookID, requester uuid.UUID) error
	DisableWebhook(ctx context.Context, webhookID, requester uuid.UUID) error
	DeleteWebhook(ctx context.Context, webhookID, requester uuid.UUID) error
	TriggerTest(ctx context.Context, webhookID, requester uuid.UUID, eventType string) (*Delivery, error)
	ListDeliveries(ctx context.Context, webhookID, requester uuid.UUID, filter DeliveryFilter) ([]*Delivery, *Cursor, error)
	GetDelivery(ctx context.Context, deliveryID, requester uuid.UUID) (*Delivery, error)
}

type DispatcherService interface {
	EnqueueForEvent(ctx context.Context, env Envelope) error
}

// ── Repository interface (defined here; implemented in repository/) ────────────

type Repository interface {
	// Webhooks
	CreateWebhook(ctx context.Context, wh *Webhook) (*Webhook, error)
	GetWebhook(ctx context.Context, id uuid.UUID) (*Webhook, error)
	ListWebhooksByProject(ctx context.Context, projectID uuid.UUID) ([]*Webhook, error)
	ListActiveWebhooksMatchingEvent(ctx context.Context, projectID uuid.UUID, eventType string) ([]*Webhook, error)
	UpdateWebhook(ctx context.Context, id uuid.UUID, patch WebhookPatch) (*Webhook, error)
	UpdateWebhookSecret(ctx context.Context, id uuid.UUID, hashedSecret string) (*Webhook, error)
	SetWebhookActive(ctx context.Context, id uuid.UUID, active bool) error
	DeleteWebhook(ctx context.Context, id uuid.UUID) error
	DeleteWebhooksByProject(ctx context.Context, projectID uuid.UUID) error

	// Deliveries
	CreateDelivery(ctx context.Context, d *Delivery) (*Delivery, error)
	PickNextPendingDelivery(ctx context.Context) (*Delivery, error)
	GetDelivery(ctx context.Context, id uuid.UUID) (*Delivery, error)
	ListDeliveriesByWebhook(ctx context.Context, webhookID uuid.UUID, filter DeliveryFilter) ([]*Delivery, error)
	MarkDeliveryDelivered(ctx context.Context, id uuid.UUID, statusCode int, body string, durationMs int) error
	RescheduleDelivery(ctx context.Context, id uuid.UUID, nextAt time.Time, statusCode *int, body, errMsg *string, durationMs int) error
	MarkDeliveryDead(ctx context.Context, id uuid.UUID, reason string) error
	MarkPendingDeliveriesDeadByWebhook(ctx context.Context, webhookID uuid.UUID) error
	DeleteOldDeliveries(ctx context.Context, before time.Time) (int64, error)

	// Known projects
	UpsertKnownProject(ctx context.Context, id uuid.UUID) error
	KnownProjectExists(ctx context.Context, id uuid.UUID) (bool, error)
	MarkKnownProjectDeleted(ctx context.Context, id uuid.UUID) error

	// Outbox
	InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error
	GetUnpublishedEvents(ctx context.Context, limit int32) ([]*OutboxEvent, error)
	MarkEventPublished(ctx context.Context, id uuid.UUID) error

	// Processed events
	WasEventProcessed(ctx context.Context, eventID string) (bool, error)
	MarkEventProcessed(ctx context.Context, eventID string) error
}

// ── Permission checker port ───────────────────────────────────────────────────

type PermissionChecker interface {
	HasPermission(ctx context.Context, projectID, userID uuid.UUID, permission string) (bool, error)
}
