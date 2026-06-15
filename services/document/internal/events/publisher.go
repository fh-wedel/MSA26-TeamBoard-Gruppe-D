package events

import (
	"context"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/teamboard/services/document/internal/domain"
)

const (
	defaultBatchSize = 50
	defaultInterval  = 200 * time.Millisecond
)

type Publisher struct {
	repo     domain.Repository
	conn     *amqp.Connection
	exchange string
	interval time.Duration
	batch    int32
}

func NewPublisher(repo domain.Repository, conn *amqp.Connection, exchange string) *Publisher {
	return &Publisher{
		repo:     repo,
		conn:     conn,
		exchange: exchange,
		interval: defaultInterval,
		batch:    defaultBatchSize,
	}
}

func (p *Publisher) Run(ctx context.Context) {
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

	evts, err := p.repo.GetUnpublishedEvents(ctx, p.batch)
	if err != nil {
		return err
	}

	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, len(evts)+1))

	for _, e := range evts {
		if err := ch.PublishWithContext(ctx, p.exchange, e.EventType, false, false, amqp.Publishing{
			ContentType:  "application/json",
			Body:         e.Payload,
			MessageId:    e.ID.String(),
			DeliveryMode: amqp.Persistent,
		}); err != nil {
			return err
		}
	}

	for i := range evts {
		confirm := <-confirms
		if confirm.Ack {
			_ = p.repo.MarkEventPublished(ctx, evts[i].ID)
		}
	}
	return nil
}
