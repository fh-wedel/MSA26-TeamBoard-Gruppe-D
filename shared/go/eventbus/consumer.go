package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Handler is called for each consumed event. Return a non-nil error to nack and requeue.
type Handler func(ctx context.Context, env Envelope) error

// Consumer subscribes to a queue and dispatches events to a Handler.
type Consumer interface {
	Subscribe(ctx context.Context, handler Handler) error
	Close() error
}

type consumer struct {
	conn  *amqp.Connection
	ch    *amqp.Channel
	queue string
}

// NewConsumer creates a Consumer bound to queue with the given topology options.
func NewConsumer(conn *amqp.Connection, opts TopologyOptions) (Consumer, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open channel: %w", err)
	}
	if err := ch.Qos(1, 0, false); err != nil {
		return nil, fmt.Errorf("set QoS: %w", err)
	}
	if err := SetupTopology(ch, opts); err != nil {
		return nil, fmt.Errorf("setup topology: %w", err)
	}
	return &consumer{conn: conn, ch: ch, queue: opts.Queue}, nil
}

func (c *consumer) Subscribe(ctx context.Context, handler Handler) error {
	deliveries, err := c.ch.Consume(c.queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume queue %q: %w", c.queue, err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("delivery channel closed")
			}
			c.handle(ctx, d, handler)
		}
	}
}

func (c *consumer) handle(ctx context.Context, d amqp.Delivery, handler Handler) {
	var env Envelope
	if err := json.Unmarshal(d.Body, &env); err != nil {
		slog.Error("failed to unmarshal envelope", "error", err)
		_ = d.Nack(false, false) // dead-letter malformed messages
		return
	}

	if err := handler(ctx, env); err != nil {
		slog.Warn("event handler error, nacking", "event_type", env.EventType, "event_id", env.EventID, "error", err)
		_ = d.Nack(false, true) // requeue for retry
		return
	}
	_ = d.Ack(false)
}

func (c *consumer) Close() error {
	return c.ch.Close()
}

// IdempotencyStore tracks which event IDs have been processed.
type IdempotencyStore interface {
	HasProcessed(ctx context.Context, eventID string) (bool, error)
	MarkProcessed(ctx context.Context, eventID string) error
}

// IdempotencyFuncs adapts a pair of functions to the IdempotencyStore interface.
// Useful for wiring repositories that expose differently named methods
// (e.g. WasEventProcessed / MarkEventProcessed).
type IdempotencyFuncs struct {
	Has  func(ctx context.Context, eventID string) (bool, error)
	Mark func(ctx context.Context, eventID string) error
}

func (f IdempotencyFuncs) HasProcessed(ctx context.Context, eventID string) (bool, error) {
	return f.Has(ctx, eventID)
}

func (f IdempotencyFuncs) MarkProcessed(ctx context.Context, eventID string) error {
	return f.Mark(ctx, eventID)
}

// IdempotentHandler wraps a Handler with processed_events deduplication.
func IdempotentHandler(inner Handler, store IdempotencyStore) Handler {
	return func(ctx context.Context, env Envelope) error {
		done, err := store.HasProcessed(ctx, env.EventID)
		if err != nil {
			return fmt.Errorf("idempotency check: %w", err)
		}
		if done {
			slog.Debug("duplicate event skipped", "event_id", env.EventID, "event_type", env.EventType)
			return nil
		}
		if err := inner(ctx, env); err != nil {
			return err
		}
		return store.MarkProcessed(ctx, env.EventID)
	}
}
