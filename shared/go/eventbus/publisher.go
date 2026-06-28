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

	// Publish with a deferred confirmation and wait for the broker ack before
	// returning. Callers (notably outbox.Worker) mark the event published in the
	// same DB transaction once Publish returns nil, so returning before the ack
	// would risk losing an event while recording it as published.
	conf, err := p.ch.PublishWithDeferredConfirmWithContext(ctx, p.exchange, env.EventType, false, false, amqp.Publishing{
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

	ok, err := conf.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("await confirm %q: %w", env.EventType, err)
	}
	if !ok {
		return fmt.Errorf("publish %q nacked by broker", env.EventType)
	}

	slog.Debug("event published", "event_type", env.EventType, "event_id", env.EventID)
	return nil
}

func (p *publisher) Close() error {
	return p.ch.Close()
}
