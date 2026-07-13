-- name: CreateColumn :one
INSERT INTO board_columns (id, board_id, name, position, wip_limit)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListColumnsByBoard :many
SELECT * FROM board_columns
WHERE board_id = $1
ORDER BY position;

-- name: DeleteColumn :exec
DELETE FROM board_columns WHERE id = $1;
