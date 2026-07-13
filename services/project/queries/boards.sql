-- name: CreateBoard :one
INSERT INTO boards (id, project_id, name, type, position, config, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetBoard :one
SELECT * FROM boards
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListBoardsByProject :many
SELECT * FROM boards
WHERE project_id = $1 AND deleted_at IS NULL
ORDER BY position;

-- name: UpdateBoard :one
UPDATE boards
SET name       = COALESCE($2, name),
    config     = COALESCE($3, config),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteBoard :exec
UPDATE boards SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetMaxBoardPosition :one
SELECT COALESCE(MAX(position), -1) FROM boards
WHERE project_id = $1 AND deleted_at IS NULL;
