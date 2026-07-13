-- name: InsertOutboxEvent :exec
INSERT INTO outbox (id, aggregate_id, event_type, payload)
VALUES ($1, $2, $3, $4);

-- name: GetUnpublishedEvents :many
SELECT * FROM outbox
WHERE published_at IS NULL
ORDER BY occurred_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkEventPublished :exec
UPDATE outbox SET published_at = NOW() WHERE id = $1;
