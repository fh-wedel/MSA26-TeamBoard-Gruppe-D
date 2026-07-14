-- name: AddMember :one
INSERT INTO project_members (project_id, user_id, role, invited_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetMember :one
SELECT * FROM project_members
WHERE project_id = $1 AND user_id = $2;

-- name: ListMembers :many
SELECT m.project_id, m.user_id, m.role, m.invited_by, m.joined_at, u.email
FROM project_members m
LEFT JOIN known_users u ON u.id = m.user_id
WHERE m.project_id = $1
ORDER BY m.joined_at;

-- name: UpdateMemberRole :one
UPDATE project_members
SET role = $3
WHERE project_id = $1 AND user_id = $2
RETURNING *;

-- name: RemoveMember :exec
DELETE FROM project_members
WHERE project_id = $1 AND user_id = $2;

-- name: CountOwners :one
SELECT COUNT(*) FROM project_members
WHERE project_id = $1 AND role = 'owner';

-- name: GetMemberRole :one
SELECT role FROM project_members
WHERE project_id = $1 AND user_id = $2;

-- name: RemoveAllMembershipsOfUser :many
DELETE FROM project_members
WHERE user_id = $1
RETURNING project_id;
