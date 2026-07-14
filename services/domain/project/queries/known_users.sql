-- name: UpsertKnownUser :exec
INSERT INTO known_users (id, email, created_at)
VALUES ($1, $2, $3)
ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email;

-- name: GetKnownUserByEmail :one
SELECT * FROM known_users
WHERE email = $1 AND deleted_at IS NULL;

-- name: GetKnownUserByID :one
SELECT * FROM known_users
WHERE id = $1 AND deleted_at IS NULL;

-- name: MarkKnownUserDeleted :exec
UPDATE known_users SET deleted_at = NOW() WHERE id = $1;
