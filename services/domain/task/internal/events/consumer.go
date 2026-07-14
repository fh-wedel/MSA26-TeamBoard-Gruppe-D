package events

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/teamboard/services/domain/task/internal/domain"
	"github.com/teamboard/shared/go/eventbus"
)

// BindingKeys lists the routing keys the task service subscribes to.
func BindingKeys() []string {
	return []string{
		"user.registered",
		"user.deleted",
		"board.created",
		"board.deleted",
		"column.created",
		"column.updated",
		"column.deleted",
		"project.deleted",
		"document.deleted",
	}
}

// EventHandler dispatches inbound domain events to the task repository.
// It is wired as the root eventbus.Handler (wrapped with IdempotentHandler).
type EventHandler struct {
	repo domain.Repository
}

// NewEventHandler creates the root handler for the task event consumer.
func NewEventHandler(repo domain.Repository) *EventHandler {
	return &EventHandler{repo: repo}
}

// Handle routes an event by type to the matching handler, passing the raw
// payload. Unknown event types are ignored.
func (h *EventHandler) Handle(ctx context.Context, env eventbus.Envelope) error {
	switch env.EventType {
	case "user.registered":
		return h.handleUserRegistered(ctx, env.Payload)
	case "user.deleted":
		return h.handleUserDeleted(ctx, env.Payload)
	case "board.created":
		return h.handleBoardCreated(ctx, env.Payload)
	case "board.deleted":
		return h.handleBoardDeleted(ctx, env.Payload)
	case "column.created", "column.updated":
		return h.handleColumnUpserted(ctx, env.Payload)
	case "column.deleted":
		return h.handleColumnDeleted(ctx, env.Payload)
	case "project.deleted":
		return h.handleProjectDeleted(ctx, env.Payload)
	case "document.deleted":
		return h.handleDocumentDeleted(ctx, env.Payload)
	}
	return nil
}

func (h *EventHandler) handleUserRegistered(ctx context.Context, body []byte) error {
	var payload struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	userID, err := uuid.Parse(payload.UserID)
	if err != nil {
		return err
	}
	return h.repo.UpsertKnownUser(ctx, userID, payload.Email)
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
	// Unassign from all tasks — returns task IDs for downstream events if needed.
	_, err = h.repo.ClearAssigneeForUser(ctx, userID)
	if err != nil {
		return err
	}
	return h.repo.MarkKnownUserDeleted(ctx, userID)
}

func (h *EventHandler) handleBoardCreated(ctx context.Context, body []byte) error {
	var payload struct {
		BoardID   string `json:"board_id"`
		ProjectID string `json:"project_id"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		Columns   []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Position int    `json:"position"`
			Status   string `json:"status"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	boardID, err := uuid.Parse(payload.BoardID)
	if err != nil {
		return err
	}
	projectID, err := uuid.Parse(payload.ProjectID)
	if err != nil {
		return err
	}
	if err := h.repo.UpsertKnownBoard(ctx, boardID, projectID, payload.Name, payload.Type); err != nil {
		return err
	}
	for _, col := range payload.Columns {
		columnID, err := uuid.Parse(col.ID)
		if err != nil {
			return err
		}
		if err := h.repo.UpsertKnownColumn(ctx, columnID, boardID, col.Name, col.Position, col.Status); err != nil {
			return err
		}
	}
	return nil
}

func (h *EventHandler) handleBoardDeleted(ctx context.Context, body []byte) error {
	var payload struct {
		BoardID string `json:"board_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	boardID, err := uuid.Parse(payload.BoardID)
	if err != nil {
		return err
	}
	if err := h.repo.MarkBoardDeleted(ctx, boardID); err != nil {
		return err
	}
	// Soft-delete all tasks for the board; outbox events are inserted by the
	// service layer, but here we cascade directly through the repository.
	_, err = h.repo.SoftDeleteTasksByBoard(ctx, boardID)
	return err
}

func (h *EventHandler) handleColumnUpserted(ctx context.Context, body []byte) error {
	var payload struct {
		ColumnID string `json:"column_id"`
		BoardID  string `json:"board_id"`
		Name     string `json:"name"`
		Position int    `json:"position"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	columnID, err := uuid.Parse(payload.ColumnID)
	if err != nil {
		return err
	}
	boardID, err := uuid.Parse(payload.BoardID)
	if err != nil {
		return err
	}
	return h.repo.UpsertKnownColumn(ctx, columnID, boardID, payload.Name, payload.Position, payload.Status)
}

func (h *EventHandler) handleColumnDeleted(ctx context.Context, body []byte) error {
	var payload struct {
		ColumnID string `json:"column_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	columnID, err := uuid.Parse(payload.ColumnID)
	if err != nil {
		return err
	}
	// Null out column_id on affected tasks (they stay on the board, status kept).
	if err := h.repo.NullifyColumnReferences(ctx, columnID); err != nil {
		return err
	}
	return h.repo.DeleteKnownColumn(ctx, columnID)
}

func (h *EventHandler) handleProjectDeleted(ctx context.Context, body []byte) error {
	var payload struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	projectID, err := uuid.Parse(payload.ProjectID)
	if err != nil {
		return err
	}
	_, err = h.repo.SoftDeleteTasksByProject(ctx, projectID)
	return err
}

func (h *EventHandler) handleDocumentDeleted(ctx context.Context, body []byte) error {
	var payload struct {
		DocumentID string `json:"document_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	documentID, err := uuid.Parse(payload.DocumentID)
	if err != nil {
		return err
	}
	_, err = h.repo.DeleteAttachmentsByDocument(ctx, documentID)
	return err
}
