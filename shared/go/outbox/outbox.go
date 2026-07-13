// Package outbox implements the transactional outbox pattern for reliable event publishing.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teamboard/shared/go/eventbus"
)

// InsertEventParams holds the data for a single outbox entry.
type InsertEventParams struct {
	ID          uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	Payload     any
}

// InsertEvent writes an event to the outbox table within an existing transaction.
// The outbox table must exist in the same database as the domain tables.
func InsertEvent(ctx context.Context, tx pgx.Tx, params InsertEventParams) error {
	payload, err := json.Marshal(params.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO outbox (id, aggregate_id, event_type, payload) VALUES ($1, $2, $3, $4)`,
		params.ID, params.AggregateID, params.EventType, payload,
	)
	return err
}

// Config holds Worker configuration.
type Config struct {
	Pool         *pgxpool.Pool
	Publisher    eventbus.Publisher
	Producer     string
	TableName    string
	PollInterval time.Duration
	BatchSize    int
}

func (c *Config) setDefaults() {
	if c.TableName == "" {
		c.TableName = "outbox"
	}
	if c.PollInterval == 0 {
		c.PollInterval = time.Second
	}
	if c.BatchSize == 0 {
		c.BatchSize = 100
	}
}

// Worker polls the outbox table and publishes pending events via the Publisher.
type Worker struct {
	cfg    Config
	logger *slog.Logger
}

// NewWorker creates a Worker with the given configuration.
func NewWorker(cfg Config) *Worker {
	cfg.setDefaults()
	return &Worker{cfg: cfg, logger: slog.Default()}
}

// Run starts the polling loop. It blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.processBatch(ctx); err != nil {
				w.logger.Warn("outbox batch failed", "error", err)
			}
		}
	}
}

type outboxRow struct {
	id          uuid.UUID
	aggregateID uuid.UUID
	eventType   string
	payload     []byte
	occurredAt  time.Time
}

func (w *Worker) processBatch(ctx context.Context) error {
	tx, err := w.cfg.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := tx.Query(ctx, fmt.Sprintf(
		`SELECT id, aggregate_id, event_type, payload, occurred_at
		 FROM %s WHERE published_at IS NULL
		 ORDER BY occurred_at ASC, id ASC
		 LIMIT $1 FOR UPDATE SKIP LOCKED`,
		w.cfg.TableName,
	), w.cfg.BatchSize)
	if err != nil {
		return fmt.Errorf("query outbox: %w", err)
	}

	var batch []outboxRow
	for rows.Next() {
		var r outboxRow
		if err := rows.Scan(&r.id, &r.aggregateID, &r.eventType, &r.payload, &r.occurredAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan row: %w", err)
		}
		batch = append(batch, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, row := range batch {
		env := eventbus.Envelope{
			EventID:       row.id.String(),
			EventType:     row.eventType,
			EventVersion:  1,
			OccurredAt:    row.occurredAt,
			Producer:      w.cfg.Producer,
			AggregateType: aggregateTypeFromEventType(row.eventType),
			AggregateID:   row.aggregateID.String(),
			Actor:         eventbus.Actor{Type: "service"},
			Payload:       row.payload,
		}
		if err := w.cfg.Publisher.Publish(ctx, env); err != nil {
			return fmt.Errorf("publish %s: %w", row.id, err)
		}
		if _, err := tx.Exec(ctx,
			fmt.Sprintf(`UPDATE %s SET published_at = NOW() WHERE id = $1`, w.cfg.TableName),
			row.id,
		); err != nil {
			return fmt.Errorf("mark published: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// aggregateTypeFromEventType extracts the aggregate type from "aggregate.event" routing keys.
func aggregateTypeFromEventType(eventType string) string {
	for i, c := range eventType {
		if c == '.' {
			return eventType[:i]
		}
	}
	return eventType
}
