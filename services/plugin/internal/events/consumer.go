package events

import (
	"context"
	"encoding/json"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/teamboard/services/plugin/internal/domain"
)

type Consumer struct {
	conn       *amqp.Connection
	exchange   string
	queue      string
	bindingKeys []string
	dispatcher domain.DispatcherService
	projectHandler *ProjectHandler
	userHandler    *UserHandler
	logger     *slog.Logger
}

func NewConsumer(
	conn *amqp.Connection,
	exchange, queue string,
	bindingKeys []string,
	dispatcher domain.DispatcherService,
	projectHandler *ProjectHandler,
	userHandler *UserHandler,
	logger *slog.Logger,
) *Consumer {
	return &Consumer{
		conn:           conn,
		exchange:       exchange,
		queue:          queue,
		bindingKeys:    bindingKeys,
		dispatcher:     dispatcher,
		projectHandler: projectHandler,
		userHandler:    userHandler,
		logger:         logger,
	}
}

func (c *Consumer) Run(ctx context.Context) error {
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
	for _, key := range c.bindingKeys {
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
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			if err := c.handle(ctx, msg); err != nil {
				c.logger.Error("event handling failed", "err", err, "routing_key", msg.RoutingKey)
				_ = msg.Nack(false, true) // requeue
			} else {
				_ = msg.Ack(false)
			}
		}
	}
}

func (c *Consumer) handle(ctx context.Context, msg amqp.Delivery) error {
	var env Envelope
	if err := json.Unmarshal(msg.Body, &env); err != nil {
		c.logger.Warn("failed to parse envelope", "err", err)
		return nil // don't requeue malformed messages
	}

	switch {
	case env.EventType == "project.created":
		return c.projectHandler.OnProjectCreated(ctx, env)
	case env.EventType == "project.deleted":
		return c.projectHandler.OnProjectDeleted(ctx, env)
	case env.EventType == "user.deleted":
		return c.userHandler.OnUserDeleted(ctx, env)
	default:
		return c.dispatcher.EnqueueForEvent(ctx, env)
	}
}
