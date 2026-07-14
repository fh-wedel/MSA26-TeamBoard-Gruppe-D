package db

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Notification struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Type          string
	Payload       json.RawMessage
	ProjectID     *uuid.UUID
	SourceEventID *string
	CreatedAt     time.Time
	ReadAt        *time.Time
}

type OutboxEvent struct {
	ID          uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	Payload     []byte
	OccurredAt  time.Time
	PublishedAt *time.Time
}
