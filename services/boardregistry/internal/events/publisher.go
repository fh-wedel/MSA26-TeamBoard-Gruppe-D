package events

import (
	"context"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/teamboard/services/boardregistry/internal/domain"
)

// Publisher polls the outbox table and publishes events to the topic exchange.
type Publisher struct {
	repo     domain.Repository
	conn     *amqp.Connection
	exchange string
	interval time.Duration
	batch    int32
}

func NewPublisher(repo domain.Repository, conn *amqp.Connection, exchange string, interval time.Duration, batch int32) *Publisher {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if batch <= 0 {
		batch = 50
	}
	return &Publisher{repo: repo, conn: conn, exchange: exchange, interval: interval, batch: batch}
}

func (p *Publisher) Run(ctx context.Context) {
	// Ensure the shared topic exchange exists even if this service boots first.
	if ch, err := p.conn.Channel(); err == nil {
		_ = ch.ExchangeDeclare(p.exchange, "topic", true, false, false, false, nil)
		_ = ch.Close()
	}

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.publishBatch(ctx); err != nil {
				slog.ErrorContext(ctx, "outbox publish error", "error", err)
			}
		}
	}
}

func (p *Publisher) publishBatch(ctx context.Context) error {
	ch, err := p.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := ch.Confirm(false); err != nil {
		return err
	}

	events, err := p.repo.GetUnpublishedEvents(ctx, p.batch)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}

	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, len(events)+1))

	for _, e := range events {
		if err := ch.PublishWithContext(ctx, p.exchange, e.EventType, false, false, amqp.Publishing{
			ContentType:  "application/json",
			Body:         e.Payload,
			MessageId:    e.ID.String(),
			DeliveryMode: amqp.Persistent,
		}); err != nil {
			return err
		}
	}

	for i := range events {
		confirm := <-confirms
		if confirm.Ack {
			_ = p.repo.MarkEventPublished(ctx, events[i].ID)
		}
	}
	return nil
}
