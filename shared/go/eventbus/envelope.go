// Package eventbus wraps RabbitMQ with a standardised event envelope and topology.
package eventbus

import (
	"encoding/json"
	"time"
)

// Envelope is the canonical event format for all TeamBoard domain events.
type Envelope struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	EventVersion  int             `json:"event_version"`
	OccurredAt    time.Time       `json:"occurred_at"`
	TraceID       string          `json:"trace_id,omitempty"`
	Producer      string          `json:"producer"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	Actor         Actor           `json:"actor"`
	Payload       json.RawMessage `json:"payload"`
}

// Actor identifies who triggered the event.
type Actor struct {
	UserID string `json:"user_id,omitempty"`
	Type   string `json:"type"` // "user", "service", "system"
}

// MustMarshalPayload marshals v into JSON, panicking on error. Use for compile-time-safe payloads.
func MustMarshalPayload(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("eventbus: marshal payload: " + err.Error())
	}
	return b
}
