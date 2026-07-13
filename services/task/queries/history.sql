-- name: CreateHistoryEntry :exec
INSERT INTO task_history (id, task_id, actor_id, change_type, diff)
VALUES ($1, $2, $3, $4, $5);

-- name: ListTaskHistory :many
SELECT * FROM task_history WHERE task_id = $1 ORDER BY occurred_at DESC LIMIT $2;
