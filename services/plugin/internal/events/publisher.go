package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/teamboard/services/plugin/internal/domain"
)

type Publisher struct {
	conn     *amqp.Connection
	exchange string
	repo     domain.Repository
	logger   *slog.Logger
}

func NewPublisher(conn *amqp.Connection, exchange string, repo domain.Repository, logger *slog.Logger) *Publisher {
	return &Publisher{conn: conn, exchange: exchange, repo: repo, logger: logger}
}

func (p *Publisher) Run(ctx context.Context) error {
	ch, err := p.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := p.publishBatch(ctx, ch); err != nil {
				p.logger.Warn("outbox publish failed", "err", err)
			}
		}
	}
}

func (p *Publisher) publishBatch(ctx context.Context, ch *amqp.Channel) error {
	events, err := p.repo.GetUnpublishedEvents(ctx, 50)
	if err != nil {
		return err
	}
	for _, e := range events {
		var payload map[string]any
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			p.logger.Warn("invalid outbox payload", "event_id", e.ID, "err", err)
			_ = p.repo.MarkEventPublished(ctx, e.ID)
			continue
		}

		body, _ := json.Marshal(map[string]any{
			"event_id":       e.ID,
			"event_type":     e.EventType,
			"aggregate_id":   e.AggregateID,
			"occurred_at":    e.OccurredAt,
			"payload":        payload,
		})

		if err := ch.PublishWithContext(ctx, p.exchange, e.EventType, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		}); err != nil {
			return err
		}
		if err := p.repo.MarkEventPublished(ctx, e.ID); err != nil {
			p.logger.Warn("failed to mark event published", "event_id", e.ID, "err", err)
		}
	}
	return nil
}
