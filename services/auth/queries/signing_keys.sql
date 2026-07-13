-- name: CreateSigningKey :one
INSERT INTO signing_keys (id, kid, algorithm, private_key_pem, public_key_pem, activated_at)
VALUES ($1, $2, $3, $4, $5, NOW())
RETURNING *;

-- name: GetActiveSigningKey :one
SELECT * FROM signing_keys
WHERE retired_at IS NULL AND deleted_at IS NULL
ORDER BY activated_at DESC
LIMIT 1;

-- name: ListValidatingSigningKeys :many
SELECT * FROM signing_keys
WHERE deleted_at IS NULL
ORDER BY activated_at DESC;

-- name: GetSigningKeyByKID :one
SELECT * FROM signing_keys
WHERE kid = $1 AND deleted_at IS NULL;

-- name: RetireSigningKey :exec
UPDATE signing_keys
SET retired_at = NOW()
WHERE id = $1 AND retired_at IS NULL;

-- name: DeleteRetiredSigningKeys :exec
UPDATE signing_keys
SET deleted_at = NOW()
WHERE retired_at IS NOT NULL
  AND retired_at < NOW() - INTERVAL '30 minutes'
  AND deleted_at IS NULL;
