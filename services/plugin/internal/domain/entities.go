package domain

import (
	"time"

	"github.com/google/uuid"
)

type Webhook struct {
	ID          uuid.UUID
	ProjectID   uuid.UUID
	TargetURL   string
	Description string
	Secret      string
	EventFilter []string
	Active      bool
	CreatedBy   uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Delivery struct {
	ID                  uuid.UUID
	WebhookID           uuid.UUID
	EventID             string
	EventType           string
	Payload             map[string]any
	Status              DeliveryStatus
	AttemptCount        int
	NextAttemptAt       time.Time
	LastResponseStatus  *int
	LastResponseBody    *string
	LastError           *string
	LastAttemptedAt     *time.Time
	DeliveredAt         *time.Time
	FailedPermanentlyAt *time.Time
	DurationMs          *int
	CreatedAt           time.Time
}

type DeliveryStatus string

const (
	DeliveryStatusPending   DeliveryStatus = "pending"
	DeliveryStatusDelivered DeliveryStatus = "delivered"
	DeliveryStatusFailed    DeliveryStatus = "failed"
	DeliveryStatusDead      DeliveryStatus = "dead"
)

type OutboxEvent struct {
	ID          uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	Payload     []byte
	OccurredAt  time.Time
	PublishedAt *time.Time
}
