-- name: RecordLoginAttempt :exec
INSERT INTO login_attempts (id, email, success, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5);

-- name: CountRecentFailedAttempts :one
SELECT COUNT(*) FROM login_attempts
WHERE email = $1
  AND success = FALSE
  AND attempted_at > NOW() - INTERVAL '15 minutes';
