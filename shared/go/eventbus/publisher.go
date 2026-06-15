package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher sends events to the RabbitMQ topic exchange.
type Publisher interface {
	Publish(ctx context.Context, env Envelope) error
	Close() error
}

type publisher struct {
	conn     *amqp.Connection
	ch       *amqp.Channel
	exchange string
}

// NewPublisher creates a Publisher on the given connection.
func NewPublisher(conn *amqp.Connection, exchange string) (Publisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open channel: %w", err)
	}
	if err := ch.Confirm(false); err != nil {
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}
	return &publisher{conn: conn, ch: ch, exchange: exchange}, nil
}

func (p *publisher) Publish(ctx context.Context, env Envelope) error {
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	traceID := ""
	if env.TraceID != "" {
		traceID = env.TraceID
	}

	err = p.ch.PublishWithContext(ctx, p.exchange, env.EventType, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
		MessageId:    env.EventID,
		Headers:      amqp.Table{"traceparent": traceID},
		Body:         body,
	})
	if err != nil {
		return fmt.Errorf("publish %q: %w", env.EventType, err)
	}

	slog.Debug("event published", "event_type", env.EventType, "event_id", env.EventID)
	return nil
}

func (p *publisher) Close() error {
	return p.ch.Close()
}
