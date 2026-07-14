-- name: UpsertKnownBoard :exec
INSERT INTO known_boards (id, project_id, name, type)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, type = EXCLUDED.type;

-- name: GetKnownBoard :one
SELECT * FROM known_boards WHERE id = $1 AND deleted_at IS NULL;

-- name: MarkBoardDeleted :exec
UPDATE known_boards SET deleted_at = NOW() WHERE id = $1;
