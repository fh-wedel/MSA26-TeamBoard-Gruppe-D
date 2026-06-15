package events

import (
	"context"
	"encoding/json"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/teamboard/services/document/internal/domain"
)

// Consumer handles inbound RabbitMQ events relevant to the Document Service.
// Routing keys consumed: project.created, project.deleted,
//                        project.member.joined, project.member.left,
//                        user.created, user.deleted
type Consumer struct {
	repo     domain.Repository
	conn     *amqp.Connection
	q        string // AMQP queue name
	exchange string
}

func NewConsumer(repo domain.Repository, conn *amqp.Connection, queueName, exchange string) *Consumer {
	return &Consumer{repo: repo, conn: conn, q: queueName, exchange: exchange}
}

func (c *Consumer) Run(ctx context.Context) {
	ch, err := c.conn.Channel()
	if err != nil {
		slog.ErrorContext(ctx, "consumer channel open failed", "error", err)
		return
	}
	defer ch.Close()

	if _, err := ch.QueueDeclare(c.q, true, false, false, false, nil); err != nil {
		slog.ErrorContext(ctx, "consumer queue declare failed", "error", err)
		return
	}
	if err := ch.QueueBind(c.q, "#", c.exchange, false, nil); err != nil {
		slog.ErrorContext(ctx, "consumer queue bind failed", "error", err)
		return
	}

	msgs, err := ch.Consume(c.q, "", false, false, false, false, nil)
	if err != nil {
		slog.ErrorContext(ctx, "consumer consume failed", "error", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgs:
			if !ok {
				return
			}
			c.handle(ctx, msg)
		}
	}
}

func (c *Consumer) handle(ctx context.Context, msg amqp.Delivery) {
	eventID := msg.MessageId
	if eventID == "" {
		_ = msg.Nack(false, false)
		return
	}

	already, err := c.repo.WasEventProcessed(ctx, eventID)
	if err != nil || already {
		_ = msg.Ack(false)
		return
	}

	var dispatchErr error
	switch msg.RoutingKey {
	case "project.created":
		dispatchErr = c.onProjectCreated(ctx, msg.Body)
	case "project.deleted":
		dispatchErr = c.onProjectDeleted(ctx, msg.Body)
	case "project.member.joined", "project.member.left":
		// no action needed — permission cache TTL handles this
		dispatchErr = nil
	case "user.created":
		dispatchErr = c.onUserCreated(ctx, msg.Body)
	case "user.deleted":
		dispatchErr = c.onUserDeleted(ctx, msg.Body)
	default:
		_ = msg.Ack(false)
		return
	}

	if dispatchErr != nil {
		slog.ErrorContext(ctx, "event handling failed", "routing_key", msg.RoutingKey, "event_id", eventID, "error", dispatchErr)
		_ = msg.Nack(false, true)
		return
	}

	_ = c.repo.MarkEventProcessed(ctx, eventID)
	_ = msg.Ack(false)
}

func (c *Consumer) onProjectCreated(ctx context.Context, body []byte) error {
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
	return c.repo.UpsertKnownProject(ctx, id)
}

func (c *Consumer) onProjectDeleted(ctx context.Context, body []byte) error {
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
	_, err = c.repo.SoftDeleteDocumentsByProject(ctx, id)
	if err != nil {
		return err
	}
	return c.repo.MarkKnownProjectDeleted(ctx, id)
}

func (c *Consumer) onUserCreated(ctx context.Context, body []byte) error {
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
	return c.repo.UpsertKnownUser(ctx, id)
}

func (c *Consumer) onUserDeleted(ctx context.Context, body []byte) error {
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
	return c.repo.MarkKnownUserDeleted(ctx, id)
}
