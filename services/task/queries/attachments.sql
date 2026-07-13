-- name: CreateAttachment :one
INSERT INTO task_attachments (id, task_id, document_id, added_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAttachment :one
SELECT * FROM task_attachments WHERE id = $1;

-- name: ListAttachmentsByTask :many
SELECT * FROM task_attachments WHERE task_id = $1 ORDER BY added_at ASC;

-- name: DeleteAttachment :exec
DELETE FROM task_attachments WHERE id = $1;

-- name: DeleteAttachmentsByDocument :many
DELETE FROM task_attachments WHERE document_id = $1 RETURNING id, task_id;
