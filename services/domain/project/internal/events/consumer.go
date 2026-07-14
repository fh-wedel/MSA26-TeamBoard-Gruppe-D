package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/teamboard/services/domain/project/internal/domain"
	"github.com/teamboard/shared/go/eventbus"
)

// BoardTypeInvalidator drops cached board-type definitions when the registry
// announces a change (implemented by boardtypeclient).
type BoardTypeInvalidator interface {
	Invalidate(typeID string)
}

// EventHandler dispatches inbound domain events to the project repository.
// It is wired as the root eventbus.Handler (wrapped with IdempotentHandler).
type EventHandler struct {
	repo        domain.Repository
	invalidator BoardTypeInvalidator
}

// NewEventHandler creates the root handler for the project event consumer.
func NewEventHandler(repo domain.Repository, invalidator BoardTypeInvalidator) *EventHandler {
	return &EventHandler{repo: repo, invalidator: invalidator}
}

// BindingKeys lists the routing keys the project service subscribes to.
func BindingKeys() []string {
	return []string{
		"user.registered", "user.deleted",
		"boardtype.registered", "boardtype.updated", "boardtype.deleted",
	}
}

// Handle routes an event by type and passes the raw payload to the matching
// handler. Unknown event types are ignored.
func (h *EventHandler) Handle(ctx context.Context, env eventbus.Envelope) error {
	switch env.EventType {
	case "user.registered":
		return h.handleUserRegistered(ctx, env.Payload)
	case "user.deleted":
		return h.handleUserDeleted(ctx, env.Payload)
	case "boardtype.registered", "boardtype.updated", "boardtype.deleted":
		return h.handleBoardTypeChanged(env.Payload)
	}
	return nil
}

func (h *EventHandler) handleUserRegistered(ctx context.Context, body []byte) error {
	var payload struct {
		UserID    string `json:"user_id"`
		Email     string `json:"email"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	userID, err := uuid.Parse(payload.UserID)
	if err != nil {
		return err
	}
	createdAt, err := time.Parse(time.RFC3339, payload.CreatedAt)
	if err != nil {
		createdAt = time.Now()
	}
	return h.repo.UpsertKnownUser(ctx, userID, payload.Email, createdAt)
}

func (h *EventHandler) handleUserDeleted(ctx context.Context, body []byte) error {
	var payload struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	userID, err := uuid.Parse(payload.UserID)
	if err != nil {
		return err
	}

	if _, err := h.repo.RemoveAllMembershipsOfUser(ctx, userID); err != nil {
		return err
	}

	return h.repo.MarkKnownUserDeleted(ctx, userID)
}

func (h *EventHandler) handleBoardTypeChanged(body []byte) error {
	if h.invalidator == nil {
		return nil
	}
	var payload struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	if payload.Type != "" {
		h.invalidator.Invalidate(payload.Type)
	}
	return nil
}
