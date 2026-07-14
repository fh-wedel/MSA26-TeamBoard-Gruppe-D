-- name: CreateComment :one
INSERT INTO task_comments (id, task_id, author_id, body)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetComment :one
SELECT * FROM task_comments WHERE id = $1 AND deleted_at IS NULL;

-- name: ListCommentsByTask :many
SELECT * FROM task_comments WHERE task_id = $1 AND deleted_at IS NULL ORDER BY created_at ASC;

-- name: UpdateComment :one
UPDATE task_comments SET body = $2, edited_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteComment :exec
UPDATE task_comments SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL;

-- name: ArchiveCommentBody :exec
INSERT INTO comment_history (id, comment_id, body) VALUES ($1, $2, $3);

-- name: AddMention :exec
INSERT INTO comment_mentions (comment_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ListMentionsByComment :many
SELECT user_id FROM comment_mentions WHERE comment_id = $1;

-- name: ClearMentions :exec
DELETE FROM comment_mentions WHERE comment_id = $1;
