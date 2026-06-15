package events

import (
	"context"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/teamboard/services/notification/internal/domain"
)

type Consumer struct {
	repo       domain.Repository
	dispatcher *Dispatcher
	conn       *amqp.Connection
	queueName  string
	exchange   string
}

func NewConsumer(repo domain.Repository, dispatcher *Dispatcher, conn *amqp.Connection, queueName, exchange string) *Consumer {
	return &Consumer{repo: repo, dispatcher: dispatcher, conn: conn, queueName: queueName, exchange: exchange}
}

func (c *Consumer) Run(ctx context.Context) {
	ch, err := c.conn.Channel()
	if err != nil {
		slog.ErrorContext(ctx, "consumer channel open failed", "error", err)
		return
	}
	defer ch.Close()

	if _, err := ch.QueueDeclare(c.queueName, true, false, false, false, nil); err != nil {
		slog.ErrorContext(ctx, "consumer queue declare failed", "error", err)
		return
	}
	if err := ch.QueueBind(c.queueName, "#", c.exchange, false, nil); err != nil {
		slog.ErrorContext(ctx, "consumer queue bind failed", "error", err)
		return
	}

	msgs, err := ch.Consume(c.queueName, "", false, false, false, false, nil)
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

	env := Envelope{
		EventType: msg.RoutingKey,
		MessageID: eventID,
		Payload:   msg.Body,
	}

	if dispErr := c.dispatcher.Dispatch(ctx, env); dispErr != nil {
		slog.ErrorContext(ctx, "event dispatch failed", "routing_key", msg.RoutingKey, "event_id", eventID, "error", dispErr)
		_ = msg.Nack(false, true)
		return
	}

	_ = c.repo.MarkEventProcessed(ctx, eventID)
	_ = msg.Ack(false)
}
