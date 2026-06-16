-- name: CreateTask :one
INSERT INTO tasks (
    id, board_id, project_id, column_id, title, description,
    status, priority, assignee_id, due_date, start_date, labels, position, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1 AND deleted_at IS NULL;

-- name: GetTaskWithCounts :one
SELECT
    t.*,
    (SELECT COUNT(*) FROM task_comments c WHERE c.task_id = t.id AND c.deleted_at IS NULL)::INT AS comment_count,
    (SELECT COUNT(*) FROM task_attachments a WHERE a.task_id = t.id)::INT AS attachment_count
FROM tasks t
WHERE t.id = $1 AND t.deleted_at IS NULL;

-- name: ListTasksByBoard :many
SELECT id, board_id, project_id, column_id, title, description,
       status, priority, assignee_id, due_date, start_date, labels, position, created_by,
       created_at, updated_at, deleted_at
FROM tasks
WHERE board_id = $1 AND deleted_at IS NULL
  AND ($2::TEXT IS NULL OR status = $2)
  AND ($3::UUID IS NULL OR assignee_id = $3)
  AND ($4::UUID IS NULL OR column_id = $4)
  AND ($5::TEXT IS NULL OR $5 = ANY(labels))
  AND ($6::TEXT IS NULL OR position > $6)
ORDER BY position ASC, id ASC
LIMIT $7;

-- name: GetLastPositionInColumn :one
SELECT COALESCE(MAX(position), '') FROM tasks
WHERE board_id = $1 AND (($2::UUID IS NULL AND column_id IS NULL) OR column_id = $2) AND deleted_at IS NULL;

-- name: GetPositionsByIDs :many
SELECT id, position FROM tasks WHERE id = ANY($1::UUID[]) AND deleted_at IS NULL;

-- name: UpdateTask :one
UPDATE tasks SET
    title       = COALESCE($2, title),
    description = COALESCE($3, description),
    priority    = COALESCE($4, priority),
    due_date    = CASE WHEN $5::BOOLEAN THEN $6 ELSE due_date END,
    start_date  = CASE WHEN $7::BOOLEAN THEN $8 ELSE start_date END,
    labels      = COALESCE($9, labels),
    updated_at  = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: MoveTask :one
UPDATE tasks SET
    column_id  = $2,
    position   = $3,
    status     = $4,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: AssignTask :one
UPDATE tasks SET
    assignee_id = $2,
    updated_at  = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteTask :exec
UPDATE tasks SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteTasksByProject :many
UPDATE tasks SET deleted_at = NOW(), updated_at = NOW()
WHERE project_id = $1 AND deleted_at IS NULL
RETURNING id, project_id;

-- name: SoftDeleteTasksByBoard :many
UPDATE tasks SET deleted_at = NOW(), updated_at = NOW()
WHERE board_id = $1 AND deleted_at IS NULL
RETURNING id, project_id;

-- name: ClearAssigneeForUser :many
UPDATE tasks SET assignee_id = NULL, updated_at = NOW()
WHERE assignee_id = $1 AND deleted_at IS NULL
RETURNING id, project_id;

-- name: NullifyColumnReferences :exec
UPDATE tasks SET column_id = NULL, updated_at = NOW()
WHERE column_id = $1;
