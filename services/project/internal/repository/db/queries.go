package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ── Projects ──────────────────────────────────────────────────────────────────

const createProject = `
INSERT INTO projects (id, name, description, owner_id)
VALUES ($1, $2, $3, $4)
RETURNING id, name, description, owner_id, created_at, updated_at, deleted_at`

func (q *queries) CreateProject(ctx context.Context, id, ownerID uuid.UUID, name, description string) (*Project, error) {
	row := q.db.QueryRow(ctx, createProject, id, name, description, ownerID)
	return scanProject(row)
}

const getProject = `
SELECT id, name, description, owner_id, created_at, updated_at, deleted_at
FROM projects WHERE id = $1 AND deleted_at IS NULL`

func (q *queries) GetProject(ctx context.Context, id uuid.UUID) (*Project, error) {
	return scanProject(q.db.QueryRow(ctx, getProject, id))
}

const listProjectsForUser = `
SELECT p.id, p.name, p.description, p.owner_id, p.created_at, p.updated_at, p.deleted_at
FROM projects p
INNER JOIN project_members m ON m.project_id = p.id
WHERE m.user_id = $1 AND p.deleted_at IS NULL
ORDER BY p.created_at DESC`

func (q *queries) ListProjectsForUser(ctx context.Context, userID uuid.UUID) ([]*Project, error) {
	rows, err := q.db.Query(ctx, listProjectsForUser, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Project
	for rows.Next() {
		p, err := scanProjectRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

const updateProject = `
UPDATE projects
SET name = COALESCE($2, name), description = COALESCE($3, description), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, name, description, owner_id, created_at, updated_at, deleted_at`

func (q *queries) UpdateProject(ctx context.Context, id uuid.UUID, name, description *string) (*Project, error) {
	return scanProject(q.db.QueryRow(ctx, updateProject, id, name, description))
}

const softDeleteProject = `
UPDATE projects SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL`

func (q *queries) SoftDeleteProject(ctx context.Context, id uuid.UUID) error {
	tag, err := q.db.Exec(ctx, softDeleteProject, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ── Members ───────────────────────────────────────────────────────────────────

const addMember = `
INSERT INTO project_members (project_id, user_id, role, invited_by)
VALUES ($1, $2, $3, $4)
RETURNING project_id, user_id, role, invited_by, joined_at`

func (q *queries) AddMember(ctx context.Context, projectID, userID uuid.UUID, role string, invitedBy *uuid.UUID) (*ProjectMember, error) {
	return scanMember(q.db.QueryRow(ctx, addMember, projectID, userID, role, invitedBy))
}

const getMember = `
SELECT project_id, user_id, role, invited_by, joined_at
FROM project_members WHERE project_id = $1 AND user_id = $2`

func (q *queries) GetMember(ctx context.Context, projectID, userID uuid.UUID) (*ProjectMember, error) {
	return scanMember(q.db.QueryRow(ctx, getMember, projectID, userID))
}

const listMembers = `
SELECT m.project_id, m.user_id, m.role, m.invited_by, m.joined_at, u.email
FROM project_members m
LEFT JOIN known_users u ON u.id = m.user_id
WHERE m.project_id = $1
ORDER BY m.joined_at`

func (q *queries) ListMembers(ctx context.Context, projectID uuid.UUID) ([]*ProjectMemberWithEmail, error) {
	rows, err := q.db.Query(ctx, listMembers, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ProjectMemberWithEmail
	for rows.Next() {
		m := &ProjectMemberWithEmail{}
		if err := rows.Scan(&m.ProjectID, &m.UserID, &m.Role, &m.InvitedBy, &m.JoinedAt, &m.Email); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

const updateMemberRole = `
UPDATE project_members SET role = $3
WHERE project_id = $1 AND user_id = $2
RETURNING project_id, user_id, role, invited_by, joined_at`

func (q *queries) UpdateMemberRole(ctx context.Context, projectID, userID uuid.UUID, role string) (*ProjectMember, error) {
	return scanMember(q.db.QueryRow(ctx, updateMemberRole, projectID, userID, role))
}

const removeMember = `DELETE FROM project_members WHERE project_id = $1 AND user_id = $2`

func (q *queries) RemoveMember(ctx context.Context, projectID, userID uuid.UUID) error {
	_, err := q.db.Exec(ctx, removeMember, projectID, userID)
	return err
}

const countOwners = `SELECT COUNT(*) FROM project_members WHERE project_id = $1 AND role = 'owner'`

func (q *queries) CountOwners(ctx context.Context, projectID uuid.UUID) (int64, error) {
	var n int64
	err := q.db.QueryRow(ctx, countOwners, projectID).Scan(&n)
	return n, err
}

const getMemberRole = `SELECT role FROM project_members WHERE project_id = $1 AND user_id = $2`

func (q *queries) GetMemberRole(ctx context.Context, projectID, userID uuid.UUID) (string, error) {
	var role string
	err := q.db.QueryRow(ctx, getMemberRole, projectID, userID).Scan(&role)
	return role, err
}

const removeAllMembershipsOfUser = `DELETE FROM project_members WHERE user_id = $1 RETURNING project_id`

func (q *queries) RemoveAllMembershipsOfUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.db.Query(ctx, removeAllMembershipsOfUser, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ── Boards ────────────────────────────────────────────────────────────────────

const createBoard = `
INSERT INTO boards (id, project_id, name, type, position, config, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, project_id, name, type, position, config, created_by, created_at, updated_at, deleted_at`

func (q *queries) CreateBoard(ctx context.Context, id, projectID uuid.UUID, name, bType string, position int, config []byte, createdBy uuid.UUID) (*Board, error) {
	return scanBoard(q.db.QueryRow(ctx, createBoard, id, projectID, name, bType, position, config, createdBy))
}

const getBoard = `
SELECT id, project_id, name, type, position, config, created_by, created_at, updated_at, deleted_at
FROM boards WHERE id = $1 AND deleted_at IS NULL`

func (q *queries) GetBoard(ctx context.Context, id uuid.UUID) (*Board, error) {
	return scanBoard(q.db.QueryRow(ctx, getBoard, id))
}

const listBoardsByProject = `
SELECT id, project_id, name, type, position, config, created_by, created_at, updated_at, deleted_at
FROM boards WHERE project_id = $1 AND deleted_at IS NULL ORDER BY position`

func (q *queries) ListBoardsByProject(ctx context.Context, projectID uuid.UUID) ([]*Board, error) {
	rows, err := q.db.Query(ctx, listBoardsByProject, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Board
	for rows.Next() {
		b, err := scanBoardRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

const updateBoard = `
UPDATE boards
SET name = COALESCE($2, name), config = COALESCE($3, config), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, project_id, name, type, position, config, created_by, created_at, updated_at, deleted_at`

func (q *queries) UpdateBoard(ctx context.Context, id uuid.UUID, name *string, config []byte) (*Board, error) {
	return scanBoard(q.db.QueryRow(ctx, updateBoard, id, name, config))
}

const softDeleteBoard = `UPDATE boards SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`

func (q *queries) SoftDeleteBoard(ctx context.Context, id uuid.UUID) error {
	tag, err := q.db.Exec(ctx, softDeleteBoard, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

const getMaxBoardPosition = `SELECT COALESCE(MAX(position), -1) FROM boards WHERE project_id = $1 AND deleted_at IS NULL`

func (q *queries) GetMaxBoardPosition(ctx context.Context, projectID uuid.UUID) (int, error) {
	var pos int
	err := q.db.QueryRow(ctx, getMaxBoardPosition, projectID).Scan(&pos)
	return pos, err
}

// ── Columns ───────────────────────────────────────────────────────────────────

const createColumn = `
INSERT INTO board_columns (id, board_id, name, position, wip_limit)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, board_id, name, position, wip_limit, created_at`

func (q *queries) CreateColumn(ctx context.Context, id, boardID uuid.UUID, name string, position int, wipLimit *int) (*BoardColumn, error) {
	return scanColumn(q.db.QueryRow(ctx, createColumn, id, boardID, name, position, wipLimit))
}

const listColumnsByBoard = `
SELECT id, board_id, name, position, wip_limit, created_at
FROM board_columns WHERE board_id = $1 ORDER BY position`

func (q *queries) ListColumnsByBoard(ctx context.Context, boardID uuid.UUID) ([]BoardColumn, error) {
	rows, err := q.db.Query(ctx, listColumnsByBoard, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BoardColumn
	for rows.Next() {
		c, err := scanColumnRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ── Known users ───────────────────────────────────────────────────────────────

const upsertKnownUser = `
INSERT INTO known_users (id, email, created_at)
VALUES ($1, $2, $3)
ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email`

func (q *queries) UpsertKnownUser(ctx context.Context, id uuid.UUID, email string, createdAt time.Time) error {
	_, err := q.db.Exec(ctx, upsertKnownUser, id, email, createdAt)
	return err
}

const getKnownUserByEmail = `
SELECT id, email, created_at, deleted_at FROM known_users
WHERE email = $1 AND deleted_at IS NULL`

func (q *queries) GetKnownUserByEmail(ctx context.Context, email string) (*KnownUser, error) {
	return scanKnownUser(q.db.QueryRow(ctx, getKnownUserByEmail, email))
}

const getKnownUserByID = `
SELECT id, email, created_at, deleted_at FROM known_users
WHERE id = $1 AND deleted_at IS NULL`

func (q *queries) GetKnownUserByID(ctx context.Context, id uuid.UUID) (*KnownUser, error) {
	return scanKnownUser(q.db.QueryRow(ctx, getKnownUserByID, id))
}

const markKnownUserDeleted = `UPDATE known_users SET deleted_at = NOW() WHERE id = $1`

func (q *queries) MarkKnownUserDeleted(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, markKnownUserDeleted, id)
	return err
}

// ── Outbox ────────────────────────────────────────────────────────────────────

const insertOutboxEvent = `INSERT INTO outbox (id, aggregate_id, event_type, payload) VALUES ($1, $2, $3, $4)`

func (q *queries) InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error {
	_, err := q.db.Exec(ctx, insertOutboxEvent, id, aggregateID, eventType, payload)
	return err
}

const getUnpublishedEvents = `
SELECT id, aggregate_id, event_type, payload, occurred_at, published_at
FROM outbox WHERE published_at IS NULL ORDER BY occurred_at ASC LIMIT $1 FOR UPDATE SKIP LOCKED`

func (q *queries) GetUnpublishedEvents(ctx context.Context, limit int32) ([]*OutboxEvent, error) {
	rows, err := q.db.Query(ctx, getUnpublishedEvents, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*OutboxEvent
	for rows.Next() {
		e := &OutboxEvent{}
		if err := rows.Scan(&e.ID, &e.AggregateID, &e.EventType, &e.Payload, &e.OccurredAt, &e.PublishedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

const markEventPublished = `UPDATE outbox SET published_at = NOW() WHERE id = $1`

func (q *queries) MarkEventPublished(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, markEventPublished, id)
	return err
}

// ── Processed events ──────────────────────────────────────────────────────────

const wasEventProcessed = `SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1)`

func (q *queries) WasEventProcessed(ctx context.Context, eventID string) (bool, error) {
	var exists bool
	err := q.db.QueryRow(ctx, wasEventProcessed, eventID).Scan(&exists)
	return exists, err
}

const markEventProcessed = `INSERT INTO processed_events (event_id) VALUES ($1) ON CONFLICT DO NOTHING`

func (q *queries) MarkEventProcessed(ctx context.Context, eventID string) error {
	_, err := q.db.Exec(ctx, markEventProcessed, eventID)
	return err
}

// ── Scan helpers ──────────────────────────────────────────────────────────────

func scanProject(r pgx.Row) (*Project, error) {
	p := &Project{}
	err := r.Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return p, err
}

func scanProjectRow(r pgx.Rows) (*Project, error) {
	p := &Project{}
	err := r.Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt)
	return p, err
}

func scanMember(r pgx.Row) (*ProjectMember, error) {
	m := &ProjectMember{}
	err := r.Scan(&m.ProjectID, &m.UserID, &m.Role, &m.InvitedBy, &m.JoinedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return m, err
}

func scanBoard(r pgx.Row) (*Board, error) {
	b := &Board{}
	err := r.Scan(&b.ID, &b.ProjectID, &b.Name, &b.Type, &b.Position, &b.Config, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt, &b.DeletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return b, err
}

func scanBoardRow(r pgx.Rows) (*Board, error) {
	b := &Board{}
	err := r.Scan(&b.ID, &b.ProjectID, &b.Name, &b.Type, &b.Position, &b.Config, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt, &b.DeletedAt)
	return b, err
}

func scanColumn(r pgx.Row) (*BoardColumn, error) {
	c := &BoardColumn{}
	err := r.Scan(&c.ID, &c.BoardID, &c.Name, &c.Position, &c.WIPLimit, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return c, err
}

func scanColumnRow(r pgx.Rows) (BoardColumn, error) {
	c := BoardColumn{}
	err := r.Scan(&c.ID, &c.BoardID, &c.Name, &c.Position, &c.WIPLimit, &c.CreatedAt)
	return c, err
}

func scanKnownUser(r pgx.Row) (*KnownUser, error) {
	u := &KnownUser{}
	err := r.Scan(&u.ID, &u.Email, &u.CreatedAt, &u.DeletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return u, err
}
