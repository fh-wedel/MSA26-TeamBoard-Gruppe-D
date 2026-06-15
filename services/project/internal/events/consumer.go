package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/teamboard/services/project/internal/domain"
)

// Consumer handles inbound RabbitMQ events.
type Consumer struct {
	repo     domain.Repository
	conn     *amqp.Connection
	queue    string
	exchange string
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
	for _, key := range []string{"user.registered", "user.deleted"} {
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
	return c.repo.UpsertKnownUser(ctx, userID, payload.Email, createdAt)
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

	projectIDs, err := c.repo.RemoveAllMembershipsOfUser(ctx, userID)
	if err != nil {
		return err
	}
	_ = projectIDs

	return c.repo.MarkKnownUserDeleted(ctx, userID)
}

// ensure domain.Repository satisfies the outbox methods we need
var _ interface {
	WasEventProcessed(context.Context, string) (bool, error)
	MarkEventProcessed(context.Context, string) error
	UpsertKnownUser(context.Context, uuid.UUID, string, time.Time) error
	MarkKnownUserDeleted(context.Context, uuid.UUID) error
	RemoveAllMembershipsOfUser(context.Context, uuid.UUID) ([]uuid.UUID, error)
} = (domain.Repository)(nil)
