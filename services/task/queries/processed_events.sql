-- name: WasEventProcessed :one
SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1);

-- name: MarkEventProcessed :exec
INSERT INTO processed_events (event_id) VALUES ($1) ON CONFLICT DO NOTHING;
