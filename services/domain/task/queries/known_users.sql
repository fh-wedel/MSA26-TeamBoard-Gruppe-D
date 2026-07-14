-- name: UpsertKnownUser :exec
INSERT INTO known_users (id, email)
VALUES ($1, $2)
ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email;

-- name: KnownUserExists :one
SELECT EXISTS(SELECT 1 FROM known_users WHERE id = $1 AND deleted_at IS NULL);

-- name: MarkKnownUserDeleted :exec
UPDATE known_users SET deleted_at = NOW() WHERE id = $1;
