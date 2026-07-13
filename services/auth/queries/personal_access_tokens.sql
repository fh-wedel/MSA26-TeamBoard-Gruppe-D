-- name: CreatePersonalAccessToken :one
INSERT INTO personal_access_tokens (id, user_id, name, token_hash, token_prefix, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetPersonalAccessTokenByHash :one
SELECT * FROM personal_access_tokens
WHERE token_hash = $1;

-- name: ListPersonalAccessTokensByUser :many
SELECT * FROM personal_access_tokens
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: RevokePersonalAccessToken :exec
UPDATE personal_access_tokens
SET revoked_at = NOW()
WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL;

-- name: TouchPersonalAccessTokenLastUsed :exec
UPDATE personal_access_tokens
SET last_used_at = NOW()
WHERE id = $1;
