package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/teamboard/services/auth/internal/repository/db"
)

// Publisher polls the outbox table and publishes events to RabbitMQ.
type Publisher struct {
	queries  *db.Queries
	conn     *amqp.Connection
	exchange string
	interval time.Duration
	batch    int
}

// NewPublisher creates a Publisher for the auth service outbox.
func NewPublisher(queries *db.Queries, conn *amqp.Connection, exchange string, interval time.Duration, batch int) *Publisher {
	return &Publisher{
		queries:  queries,
		conn:     conn,
		exchange: exchange,
		interval: interval,
		batch:    batch,
	}
}

// Run polls the outbox at the configured interval until ctx is cancelled.
func (p *Publisher) Run(ctx context.Context) error {
	ch, err := p.conn.Channel()
	if err != nil {
		return fmt.Errorf("open AMQP channel: %w", err)
	}
	defer ch.Close()

	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("enable publisher confirms: %w", err)
	}

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := p.publishBatch(ctx, ch); err != nil {
				slog.ErrorContext(ctx, "outbox publish batch failed", "error", err)
			}
		}
	}
}

func (p *Publisher) publishBatch(ctx context.Context, ch *amqp.Channel) error {
	evts, err := p.queries.GetUnpublishedEvents(ctx, int32(p.batch))
	if err != nil {
		return fmt.Errorf("fetch unpublished events: %w", err)
	}

	for _, e := range evts {
		body, _ := json.Marshal(map[string]any{
			"event_id":     e.ID,
			"event_type":   e.EventType,
			"occurred_at":  e.OccurredAt,
			"aggregate_id": e.AggregateID,
			"payload":      json.RawMessage(e.Payload),
		})

		if err := ch.PublishWithContext(ctx, p.exchange, e.EventType, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		}); err != nil {
			return fmt.Errorf("publish event %s: %w", e.ID, err)
		}

		if err := p.queries.MarkEventPublished(ctx, e.ID); err != nil {
			slog.WarnContext(ctx, "failed to mark event published", "event_id", e.ID, "error", err)
		}
	}
	return nil
}
