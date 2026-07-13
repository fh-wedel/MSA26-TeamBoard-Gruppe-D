package events

import (
	"context"
	"encoding/json"

	"github.com/teamboard/services/document/internal/domain"
	"github.com/teamboard/shared/go/eventbus"
)

// BindingKeys lists the routing keys the document service subscribes to. It
// binds to all events ("#") and ignores those it has no handler for.
func BindingKeys() []string {
	return []string{"#"}
}

// EventHandler handles inbound domain events relevant to the Document Service.
// Routing keys handled: project.created, project.deleted, user.created,
// user.deleted. It is wired as the root eventbus.Handler.
type EventHandler struct {
	repo domain.Repository
}

func NewEventHandler(repo domain.Repository) *EventHandler {
	return &EventHandler{repo: repo}
}

// Handle routes an event by type; unhandled types are ignored.
func (h *EventHandler) Handle(ctx context.Context, env eventbus.Envelope) error {
	switch env.EventType {
	case "project.created":
		return h.onProjectCreated(ctx, env.Payload)
	case "project.deleted":
		return h.onProjectDeleted(ctx, env.Payload)
	case "user.created":
		return h.onUserCreated(ctx, env.Payload)
	case "user.deleted":
		return h.onUserDeleted(ctx, env.Payload)
	}
	return nil
}

func (h *EventHandler) onProjectCreated(ctx context.Context, body []byte) error {
	var payload struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	id, err := parseUUID(payload.ProjectID)
	if err != nil {
		return err
	}
	return h.repo.UpsertKnownProject(ctx, id)
}

func (h *EventHandler) onProjectDeleted(ctx context.Context, body []byte) error {
	var payload struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	id, err := parseUUID(payload.ProjectID)
	if err != nil {
		return err
	}
	_, err = h.repo.SoftDeleteDocumentsByProject(ctx, id)
	if err != nil {
		return err
	}
	return h.repo.MarkKnownProjectDeleted(ctx, id)
}

func (h *EventHandler) onUserCreated(ctx context.Context, body []byte) error {
	var payload struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	id, err := parseUUID(payload.UserID)
	if err != nil {
		return err
	}
	return h.repo.UpsertKnownUser(ctx, id)
}

func (h *EventHandler) onUserDeleted(ctx context.Context, body []byte) error {
	var payload struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	id, err := parseUUID(payload.UserID)
	if err != nil {
		return err
	}
	return h.repo.MarkKnownUserDeleted(ctx, id)
}
