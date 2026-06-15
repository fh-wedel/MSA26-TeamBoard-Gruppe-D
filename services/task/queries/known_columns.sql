-- name: UpsertKnownColumn :exec
INSERT INTO known_columns (id, board_id, name, position)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, position = EXCLUDED.position;

-- name: GetKnownColumn :one
SELECT * FROM known_columns WHERE id = $1;

-- name: ColumnBelongsToBoard :one
SELECT EXISTS(SELECT 1 FROM known_columns WHERE id = $1 AND board_id = $2);

-- name: DeleteKnownColumn :exec
DELETE FROM known_columns WHERE id = $1;
