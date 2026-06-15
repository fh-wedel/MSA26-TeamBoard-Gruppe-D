package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/teamboard/services/task/internal/domain"
)

var routingKeys = []string{
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

// Consumer handles inbound RabbitMQ events from other services.
type Consumer struct {
	repo     domain.Repository
	conn     *amqp.Connection
	exchange string
	queue    string
}

func NewConsumer(repo domain.Repository, conn *amqp.Connection, exchange, queue string) *Consumer {
	return &Consumer{repo: repo, conn: conn, exchange: exchange, queue: queue}
}

func (c *Consumer) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := c.consume(ctx); err != nil {
			slog.ErrorContext(ctx, "consumer error, retrying", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (c *Consumer) consume(ctx context.Context) error {
	ch, err := c.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(c.exchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	q, err := ch.QueueDeclare(c.queue, true, false, false, false, nil)
	if err != nil {
		return err
	}
	for _, key := range routingKeys {
		if err := ch.QueueBind(q.Name, key, c.exchange, false, nil); err != nil {
			return err
		}
	}

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			c.handleMessage(ctx, msg)
		}
	}
}

func (c *Consumer) handleMessage(ctx context.Context, msg amqp.Delivery) {
	eventID := msg.MessageId
	if eventID == "" {
		_ = msg.Ack(false)
		return
	}

	already, err := c.repo.WasEventProcessed(ctx, eventID)
	if err != nil || already {
		_ = msg.Ack(false)
		return
	}

	var handleErr error
	switch msg.RoutingKey {
	case "user.registered":
		handleErr = c.handleUserRegistered(ctx, msg.Body)
	case "user.deleted":
		handleErr = c.handleUserDeleted(ctx, msg.Body)
	case "board.created":
		handleErr = c.handleBoardCreated(ctx, msg.Body)
	case "board.deleted":
		handleErr = c.handleBoardDeleted(ctx, msg.Body)
	case "column.created", "column.updated":
		handleErr = c.handleColumnUpserted(ctx, msg.Body)
	case "column.deleted":
		handleErr = c.handleColumnDeleted(ctx, msg.Body)
	case "project.deleted":
		handleErr = c.handleProjectDeleted(ctx, msg.Body)
	case "document.deleted":
		handleErr = c.handleDocumentDeleted(ctx, msg.Body)
	}

	if handleErr != nil {
		slog.ErrorContext(ctx, "event handler failed", "routing_key", msg.RoutingKey, "event_id", eventID, "error", handleErr)
		_ = msg.Nack(false, true)
		return
	}

	_ = c.repo.MarkEventProcessed(ctx, eventID)
	_ = msg.Ack(false)
}

func (c *Consumer) handleUserRegistered(ctx context.Context, body []byte) error {
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
	return c.repo.UpsertKnownUser(ctx, userID, payload.Email)
}

func (c *Consumer) handleUserDeleted(ctx context.Context, body []byte) error {
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
	_, err = c.repo.ClearAssigneeForUser(ctx, userID)
	if err != nil {
		return err
	}
	return c.repo.MarkKnownUserDeleted(ctx, userID)
}

func (c *Consumer) handleBoardCreated(ctx context.Context, body []byte) error {
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
	if err := c.repo.UpsertKnownBoard(ctx, boardID, projectID, payload.Name, payload.Type); err != nil {
		return err
	}
	for _, col := range payload.Columns {
		columnID, err := uuid.Parse(col.ID)
		if err != nil {
			return err
		}
		if err := c.repo.UpsertKnownColumn(ctx, columnID, boardID, col.Name, col.Position, col.Status); err != nil {
			return err
		}
	}
	return nil
}

func (c *Consumer) handleBoardDeleted(ctx context.Context, body []byte) error {
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
	if err := c.repo.MarkBoardDeleted(ctx, boardID); err != nil {
		return err
	}
	// Soft-delete all tasks for the board; outbox events are inserted by the
	// service layer, but here we cascade directly through the repository.
	_, err = c.repo.SoftDeleteTasksByBoard(ctx, boardID)
	return err
}

func (c *Consumer) handleColumnUpserted(ctx context.Context, body []byte) error {
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
	return c.repo.UpsertKnownColumn(ctx, columnID, boardID, payload.Name, payload.Position, payload.Status)
}

func (c *Consumer) handleColumnDeleted(ctx context.Context, body []byte) error {
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
	if err := c.repo.NullifyColumnReferences(ctx, columnID); err != nil {
		return err
	}
	return c.repo.DeleteKnownColumn(ctx, columnID)
}

func (c *Consumer) handleProjectDeleted(ctx context.Context, body []byte) error {
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
	_, err = c.repo.SoftDeleteTasksByProject(ctx, projectID)
	return err
}

func (c *Consumer) handleDocumentDeleted(ctx context.Context, body []byte) error {
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
	_, err = c.repo.DeleteAttachmentsByDocument(ctx, documentID)
	return err
}
