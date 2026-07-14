package events

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/teamboard/services/domain/plugin/internal/domain"
	"github.com/teamboard/shared/go/eventbus"
)

// EventHandler adapts shared eventbus envelopes to the plugin domain handlers.
// It is wired as the root eventbus.Handler (wrapped with IdempotentHandler).
type EventHandler struct {
	dispatcher     domain.DispatcherService
	projectHandler *ProjectHandler
	userHandler    *UserHandler
}

// NewEventHandler creates the root handler for the plugin event consumer.
func NewEventHandler(dispatcher domain.DispatcherService, projectHandler *ProjectHandler, userHandler *UserHandler) *EventHandler {
	return &EventHandler{dispatcher: dispatcher, projectHandler: projectHandler, userHandler: userHandler}
}

// Handle maps the shared envelope onto the plugin domain envelope and routes
// it. Project/user lifecycle events update local state; everything else is
// enqueued for webhook delivery.
func (h *EventHandler) Handle(ctx context.Context, env eventbus.Envelope) error {
	denv := toDomainEnvelope(env)
	switch denv.EventType {
	case "project.created":
		return h.projectHandler.OnProjectCreated(ctx, denv)
	case "project.deleted":
		return h.projectHandler.OnProjectDeleted(ctx, denv)
	case "user.deleted":
		return h.userHandler.OnUserDeleted(ctx, denv)
	default:
		return h.dispatcher.EnqueueForEvent(ctx, denv)
	}
}

// toDomainEnvelope converts the shared wire envelope into the plugin's domain
// envelope, decoding the raw payload into a generic map.
func toDomainEnvelope(env eventbus.Envelope) Envelope {
	var payload map[string]any
	if len(env.Payload) > 0 {
		_ = json.Unmarshal(env.Payload, &payload)
	}
	aggID, _ := uuid.Parse(env.AggregateID)
	return Envelope{
		EventID:       env.EventID,
		EventType:     env.EventType,
		OccurredAt:    env.OccurredAt,
		TraceID:       env.TraceID,
		Producer:      env.Producer,
		AggregateType: env.AggregateType,
		AggregateID:   aggID,
		Actor:         domain.Actor{UserID: env.Actor.UserID, Type: env.Actor.Type},
		Payload:       payload,
	}
}
