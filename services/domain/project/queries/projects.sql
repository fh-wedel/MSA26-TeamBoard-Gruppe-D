-- name: CreateProject :one
INSERT INTO projects (id, name, description, owner_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetProject :one
SELECT * FROM projects
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListProjectsForUser :many
SELECT p.* FROM projects p
INNER JOIN project_members m ON m.project_id = p.id
WHERE m.user_id = $1 AND p.deleted_at IS NULL
ORDER BY p.created_at DESC;

-- name: UpdateProject :one
UPDATE projects
SET name        = COALESCE($2, name),
    description = COALESCE($3, description),
    updated_at  = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteProject :exec
UPDATE projects SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;
