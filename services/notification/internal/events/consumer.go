package events

import (
	"context"

	"github.com/teamboard/shared/go/eventbus"
)

// BindingKeys lists the routing keys the notification service subscribes to.
// It binds to all events ("#"); the dispatcher ignores those without a handler.
func BindingKeys() []string {
	return []string{"#"}
}

// EventHandler adapts shared eventbus envelopes to the local dispatcher.
// It is wired as the root eventbus.Handler (wrapped with IdempotentHandler).
type EventHandler struct {
	dispatcher *Dispatcher
}

// NewEventHandler creates the root handler for the notification event consumer.
func NewEventHandler(dispatcher *Dispatcher) *EventHandler {
	return &EventHandler{dispatcher: dispatcher}
}

// Handle maps the shared envelope onto the local dispatcher envelope. The
// dispatcher resolves the handler by event type and ignores unknown types.
func (h *EventHandler) Handle(ctx context.Context, env eventbus.Envelope) error {
	return h.dispatcher.Dispatch(ctx, Envelope{
		EventType: env.EventType,
		MessageID: env.EventID,
		Payload:   env.Payload,
	})
}
