package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ── Tasks ─────────────────────────────────────────────────────────────────────

const createTask = `
INSERT INTO tasks (id, board_id, project_id, column_id, title, description,
    status, priority, assignee_id, due_date, start_date, labels, position, created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
RETURNING id,board_id,project_id,column_id,title,description,status,priority,
          assignee_id,due_date,start_date,labels,position,created_by,created_at,updated_at,deleted_at`

func (q *queries) CreateTask(ctx context.Context, id, boardID, projectID uuid.UUID, columnID *uuid.UUID, title, description, status, priority string, assigneeID *uuid.UUID, dueDate, startDate *time.Time, labels []string, position string, createdBy uuid.UUID) (*Task, error) {
	return scanTask(q.db.QueryRow(ctx, createTask, id, boardID, projectID, columnID, title, description, status, priority, assigneeID, dueDate, startDate, labels, position, createdBy))
}

const getTask = `
SELECT id,board_id,project_id,column_id,title,description,status,priority,
       assignee_id,due_date,start_date,labels,position,created_by,created_at,updated_at,deleted_at
FROM tasks WHERE id=$1 AND deleted_at IS NULL`

func (q *queries) GetTask(ctx context.Context, id uuid.UUID) (*Task, error) {
	return scanTask(q.db.QueryRow(ctx, getTask, id))
}

const getTaskWithCounts = `
SELECT t.id,t.board_id,t.project_id,t.column_id,t.title,t.description,t.status,t.priority,
       t.assignee_id,t.due_date,t.start_date,t.labels,t.position,t.created_by,t.created_at,t.updated_at,t.deleted_at,
       (SELECT COUNT(*) FROM task_comments c WHERE c.task_id=t.id AND c.deleted_at IS NULL)::INT,
       (SELECT COUNT(*) FROM task_attachments a WHERE a.task_id=t.id)::INT
FROM tasks t WHERE t.id=$1 AND t.deleted_at IS NULL`

func (q *queries) GetTaskWithCounts(ctx context.Context, id uuid.UUID) (*TaskWithCounts, error) {
	row := q.db.QueryRow(ctx, getTaskWithCounts, id)
	t := &TaskWithCounts{}
	err := row.Scan(&t.ID, &t.BoardID, &t.ProjectID, &t.ColumnID,
		&t.Title, &t.Description, &t.Status, &t.Priority,
		&t.AssigneeID, &t.DueDate, &t.StartDate, &t.Labels, &t.Position,
		&t.CreatedBy, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt,
		&t.CommentCount, &t.AttachmentCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return t, err
}

const listTasksByBoard = `
SELECT id,board_id,project_id,column_id,title,description,status,priority,
       assignee_id,due_date,start_date,labels,position,created_by,created_at,updated_at,deleted_at
FROM tasks
WHERE board_id=$1 AND deleted_at IS NULL
  AND ($2::TEXT IS NULL OR status=$2)
  AND ($3::UUID IS NULL OR assignee_id=$3)
  AND ($4::UUID IS NULL OR column_id=$4)
  AND ($5::TEXT IS NULL OR $5=ANY(labels))
  AND ($6::TEXT IS NULL OR position>$6)
ORDER BY position ASC, id ASC
LIMIT $7`

func (q *queries) ListTasksByBoard(ctx context.Context, boardID uuid.UUID, statusFilter *string, assigneeFilter *uuid.UUID, columnFilter *uuid.UUID, labelFilter *string, cursorPos *string, limit int) ([]*Task, error) {
	rows, err := q.db.Query(ctx, listTasksByBoard, boardID, statusFilter, assigneeFilter, columnFilter, labelFilter, cursorPos, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Task
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

const getLastPositionInColumn = `
SELECT COALESCE(MAX(position),'') FROM tasks
WHERE board_id=$1 AND (($2::UUID IS NULL AND column_id IS NULL) OR column_id=$2) AND deleted_at IS NULL`

func (q *queries) GetLastPositionInColumn(ctx context.Context, boardID uuid.UUID, columnID *uuid.UUID) (string, error) {
	var pos string
	err := q.db.QueryRow(ctx, getLastPositionInColumn, boardID, columnID).Scan(&pos)
	return pos, err
}

const getPositionsByIDs = `SELECT id, position FROM tasks WHERE id = ANY($1::UUID[]) AND deleted_at IS NULL`

func (q *queries) GetPositionsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	rows, err := q.db.Query(ctx, getPositionsByIDs, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[uuid.UUID]string)
	for rows.Next() {
		var id uuid.UUID
		var pos string
		if err := rows.Scan(&id, &pos); err != nil {
			return nil, err
		}
		out[id] = pos
	}
	return out, rows.Err()
}

const updateTask = `
UPDATE tasks SET
    title       = COALESCE($2,title),
    description = COALESCE($3,description),
    priority    = COALESCE($4,priority),
    due_date    = CASE WHEN $5::BOOLEAN THEN $6 ELSE due_date END,
    start_date  = CASE WHEN $7::BOOLEAN THEN $8 ELSE start_date END,
    labels      = COALESCE($9,labels),
    status      = COALESCE($10,status),
    updated_at  = NOW()
WHERE id=$1 AND deleted_at IS NULL
RETURNING id,board_id,project_id,column_id,title,description,status,priority,
          assignee_id,due_date,start_date,labels,position,created_by,created_at,updated_at,deleted_at`

func (q *queries) UpdateTask(ctx context.Context, id uuid.UUID, title, description, priority *string, dueDateSet bool, dueDate *time.Time, startDateSet bool, startDate *time.Time, labels []string, status *string) (*Task, error) {
	return scanTask(q.db.QueryRow(ctx, updateTask, id, title, description, priority, dueDateSet, dueDate, startDateSet, startDate, labels, status))
}

const moveTask = `
UPDATE tasks SET column_id=$2,position=$3,status=$4,updated_at=NOW()
WHERE id=$1 AND deleted_at IS NULL
RETURNING id,board_id,project_id,column_id,title,description,status,priority,
          assignee_id,due_date,start_date,labels,position,created_by,created_at,updated_at,deleted_at`

func (q *queries) MoveTask(ctx context.Context, id, columnID uuid.UUID, position, status string) (*Task, error) {
	return scanTask(q.db.QueryRow(ctx, moveTask, id, columnID, position, status))
}

const assignTask = `
UPDATE tasks SET assignee_id=$2,updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL
RETURNING id,board_id,project_id,column_id,title,description,status,priority,
          assignee_id,due_date,start_date,labels,position,created_by,created_at,updated_at,deleted_at`

func (q *queries) AssignTask(ctx context.Context, id uuid.UUID, assigneeID *uuid.UUID) (*Task, error) {
	return scanTask(q.db.QueryRow(ctx, assignTask, id, assigneeID))
}

const softDeleteTask = `UPDATE tasks SET deleted_at=NOW(),updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`

func (q *queries) SoftDeleteTask(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, softDeleteTask, id)
	return err
}

const softDeleteTasksByProject = `
UPDATE tasks SET deleted_at=NOW(),updated_at=NOW() WHERE project_id=$1 AND deleted_at IS NULL
RETURNING id, project_id`

func (q *queries) SoftDeleteTasksByProject(ctx context.Context, projectID uuid.UUID) ([]struct{ ID, ProjectID uuid.UUID }, error) {
	return scanIDProjectPairs(q.db.Query(ctx, softDeleteTasksByProject, projectID))
}

const softDeleteTasksByBoard = `
UPDATE tasks SET deleted_at=NOW(),updated_at=NOW() WHERE board_id=$1 AND deleted_at IS NULL
RETURNING id, project_id`

func (q *queries) SoftDeleteTasksByBoard(ctx context.Context, boardID uuid.UUID) ([]struct{ ID, ProjectID uuid.UUID }, error) {
	return scanIDProjectPairs(q.db.Query(ctx, softDeleteTasksByBoard, boardID))
}

const clearAssigneeForUser = `
UPDATE tasks SET assignee_id=NULL,updated_at=NOW() WHERE assignee_id=$1 AND deleted_at IS NULL
RETURNING id, project_id`

func (q *queries) ClearAssigneeForUser(ctx context.Context, userID uuid.UUID) ([]struct{ ID, ProjectID uuid.UUID }, error) {
	return scanIDProjectPairs(q.db.Query(ctx, clearAssigneeForUser, userID))
}

const nullifyColumnReferences = `UPDATE tasks SET column_id=NULL,updated_at=NOW() WHERE column_id=$1`

func (q *queries) NullifyColumnReferences(ctx context.Context, columnID uuid.UUID) error {
	_, err := q.db.Exec(ctx, nullifyColumnReferences, columnID)
	return err
}

// ── Comments ──────────────────────────────────────────────────────────────────

const createComment = `
INSERT INTO task_comments (id,task_id,author_id,body) VALUES($1,$2,$3,$4)
RETURNING id,task_id,author_id,body,edited_at,created_at,deleted_at`

func (q *queries) CreateComment(ctx context.Context, id, taskID, authorID uuid.UUID, body string) (*Comment, error) {
	return scanComment(q.db.QueryRow(ctx, createComment, id, taskID, authorID, body))
}

const getComment = `SELECT id,task_id,author_id,body,edited_at,created_at,deleted_at FROM task_comments WHERE id=$1 AND deleted_at IS NULL`

func (q *queries) GetComment(ctx context.Context, id uuid.UUID) (*Comment, error) {
	return scanComment(q.db.QueryRow(ctx, getComment, id))
}

const listCommentsByTask = `SELECT id,task_id,author_id,body,edited_at,created_at,deleted_at FROM task_comments WHERE task_id=$1 AND deleted_at IS NULL ORDER BY created_at ASC`

func (q *queries) ListCommentsByTask(ctx context.Context, taskID uuid.UUID) ([]*Comment, error) {
	rows, err := q.db.Query(ctx, listCommentsByTask, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Comment
	for rows.Next() {
		c, err := scanCommentRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

const updateComment = `UPDATE task_comments SET body=$2,edited_at=NOW() WHERE id=$1 AND deleted_at IS NULL RETURNING id,task_id,author_id,body,edited_at,created_at,deleted_at`

func (q *queries) UpdateComment(ctx context.Context, id uuid.UUID, body string) (*Comment, error) {
	return scanComment(q.db.QueryRow(ctx, updateComment, id, body))
}

const softDeleteComment = `UPDATE task_comments SET deleted_at=NOW() WHERE id=$1 AND deleted_at IS NULL`

func (q *queries) SoftDeleteComment(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, softDeleteComment, id)
	return err
}

const archiveCommentBody = `INSERT INTO comment_history (id,comment_id,body) VALUES($1,$2,$3)`

func (q *queries) ArchiveCommentBody(ctx context.Context, historyID, commentID uuid.UUID, body string) error {
	_, err := q.db.Exec(ctx, archiveCommentBody, historyID, commentID, body)
	return err
}

const addMention = `INSERT INTO comment_mentions (comment_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`

func (q *queries) AddMention(ctx context.Context, commentID, userID uuid.UUID) error {
	_, err := q.db.Exec(ctx, addMention, commentID, userID)
	return err
}

const listMentionsByComment = `SELECT user_id FROM comment_mentions WHERE comment_id=$1`

func (q *queries) ListMentionsByComment(ctx context.Context, commentID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.db.Query(ctx, listMentionsByComment, commentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

const clearMentions = `DELETE FROM comment_mentions WHERE comment_id=$1`

func (q *queries) ClearMentions(ctx context.Context, commentID uuid.UUID) error {
	_, err := q.db.Exec(ctx, clearMentions, commentID)
	return err
}

// ── Attachments ───────────────────────────────────────────────────────────────

const createAttachment = `INSERT INTO task_attachments (id,task_id,document_id,added_by) VALUES($1,$2,$3,$4) RETURNING id,task_id,document_id,added_by,added_at`

func (q *queries) CreateAttachment(ctx context.Context, id, taskID, documentID, addedBy uuid.UUID) (*Attachment, error) {
	return scanAttachment(q.db.QueryRow(ctx, createAttachment, id, taskID, documentID, addedBy))
}

const getAttachment = `SELECT id,task_id,document_id,added_by,added_at FROM task_attachments WHERE id=$1`

func (q *queries) GetAttachment(ctx context.Context, id uuid.UUID) (*Attachment, error) {
	return scanAttachment(q.db.QueryRow(ctx, getAttachment, id))
}

const listAttachmentsByTask = `SELECT id,task_id,document_id,added_by,added_at FROM task_attachments WHERE task_id=$1 ORDER BY added_at ASC`

func (q *queries) ListAttachmentsByTask(ctx context.Context, taskID uuid.UUID) ([]*Attachment, error) {
	rows, err := q.db.Query(ctx, listAttachmentsByTask, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Attachment
	for rows.Next() {
		a, err := scanAttachmentRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

const deleteAttachment = `DELETE FROM task_attachments WHERE id=$1`

func (q *queries) DeleteAttachment(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, deleteAttachment, id)
	return err
}

const deleteAttachmentsByDocument = `DELETE FROM task_attachments WHERE document_id=$1 RETURNING id,task_id`

func (q *queries) DeleteAttachmentsByDocument(ctx context.Context, documentID uuid.UUID) ([]struct{ ID, TaskID uuid.UUID }, error) {
	rows, err := q.db.Query(ctx, deleteAttachmentsByDocument, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct{ ID, TaskID uuid.UUID }
	for rows.Next() {
		var r struct{ ID, TaskID uuid.UUID }
		if err := rows.Scan(&r.ID, &r.TaskID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ── History ───────────────────────────────────────────────────────────────────

const createHistoryEntry = `INSERT INTO task_history (id,task_id,actor_id,change_type,diff) VALUES($1,$2,$3,$4,$5)`

func (q *queries) CreateHistoryEntry(ctx context.Context, id, taskID, actorID uuid.UUID, changeType string, diff []byte) error {
	_, err := q.db.Exec(ctx, createHistoryEntry, id, taskID, actorID, changeType, diff)
	return err
}

const listTaskHistory = `SELECT id,task_id,actor_id,change_type,diff,occurred_at FROM task_history WHERE task_id=$1 ORDER BY occurred_at DESC LIMIT $2`

func (q *queries) ListTaskHistory(ctx context.Context, taskID uuid.UUID, limit int) ([]*HistoryEntry, error) {
	rows, err := q.db.Query(ctx, listTaskHistory, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*HistoryEntry
	for rows.Next() {
		h := &HistoryEntry{}
		if err := rows.Scan(&h.ID, &h.TaskID, &h.ActorID, &h.ChangeType, &h.Diff, &h.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// ── Known boards ──────────────────────────────────────────────────────────────

const upsertKnownBoard = `INSERT INTO known_boards (id,project_id,name,type) VALUES($1,$2,$3,$4) ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name,type=EXCLUDED.type`

func (q *queries) UpsertKnownBoard(ctx context.Context, id, projectID uuid.UUID, name, bType string) error {
	_, err := q.db.Exec(ctx, upsertKnownBoard, id, projectID, name, bType)
	return err
}

const getKnownBoard = `SELECT id,project_id,name,type,deleted_at FROM known_boards WHERE id=$1 AND deleted_at IS NULL`

func (q *queries) GetKnownBoard(ctx context.Context, id uuid.UUID) (*KnownBoard, error) {
	b := &KnownBoard{}
	err := q.db.QueryRow(ctx, getKnownBoard, id).Scan(&b.ID, &b.ProjectID, &b.Name, &b.Type, &b.DeletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return b, err
}

const markBoardDeleted = `UPDATE known_boards SET deleted_at=NOW() WHERE id=$1`

func (q *queries) MarkBoardDeleted(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, markBoardDeleted, id)
	return err
}

// ── Known columns ─────────────────────────────────────────────────────────────

const upsertKnownColumn = `INSERT INTO known_columns (id,board_id,name,position,status) VALUES($1,$2,$3,$4,$5) ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name,position=EXCLUDED.position,status=EXCLUDED.status`

func (q *queries) UpsertKnownColumn(ctx context.Context, id, boardID uuid.UUID, name string, position int, status string) error {
	if status == "" {
		status = "open"
	}
	_, err := q.db.Exec(ctx, upsertKnownColumn, id, boardID, name, position, status)
	return err
}

const getKnownColumn = `SELECT id,board_id,name,position,status FROM known_columns WHERE id=$1`

func (q *queries) GetKnownColumn(ctx context.Context, id uuid.UUID) (*KnownColumn, error) {
	c := &KnownColumn{}
	err := q.db.QueryRow(ctx, getKnownColumn, id).Scan(&c.ID, &c.BoardID, &c.Name, &c.Position, &c.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return c, err
}

const columnBelongsToBoard = `SELECT EXISTS(SELECT 1 FROM known_columns WHERE id=$1 AND board_id=$2)`

func (q *queries) ColumnBelongsToBoard(ctx context.Context, columnID, boardID uuid.UUID) (bool, error) {
	var exists bool
	err := q.db.QueryRow(ctx, columnBelongsToBoard, columnID, boardID).Scan(&exists)
	return exists, err
}

const deleteKnownColumn = `DELETE FROM known_columns WHERE id=$1`

func (q *queries) DeleteKnownColumn(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, deleteKnownColumn, id)
	return err
}

// ── Known users ───────────────────────────────────────────────────────────────

const upsertKnownUser = `INSERT INTO known_users (id,email) VALUES($1,$2) ON CONFLICT (id) DO UPDATE SET email=EXCLUDED.email`

func (q *queries) UpsertKnownUser(ctx context.Context, id uuid.UUID, email string) error {
	_, err := q.db.Exec(ctx, upsertKnownUser, id, email)
	return err
}

const knownUserExists = `SELECT EXISTS(SELECT 1 FROM known_users WHERE id=$1 AND deleted_at IS NULL)`

func (q *queries) KnownUserExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := q.db.QueryRow(ctx, knownUserExists, id).Scan(&exists)
	return exists, err
}

const markKnownUserDeleted = `UPDATE known_users SET deleted_at=NOW() WHERE id=$1`

func (q *queries) MarkKnownUserDeleted(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, markKnownUserDeleted, id)
	return err
}

// ── Outbox ────────────────────────────────────────────────────────────────────

const insertOutboxEvent = `INSERT INTO outbox (id,aggregate_id,event_type,payload) VALUES($1,$2,$3,$4)`

func (q *queries) InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error {
	_, err := q.db.Exec(ctx, insertOutboxEvent, id, aggregateID, eventType, payload)
	return err
}

const getUnpublishedEvents = `SELECT id,aggregate_id,event_type,payload,occurred_at,published_at FROM outbox WHERE published_at IS NULL ORDER BY occurred_at ASC,id ASC LIMIT $1 FOR UPDATE SKIP LOCKED`

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

const markEventPublished = `UPDATE outbox SET published_at=NOW() WHERE id=$1`

func (q *queries) MarkEventPublished(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, markEventPublished, id)
	return err
}

// ── Processed events ──────────────────────────────────────────────────────────

const wasEventProcessed = `SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id=$1)`

func (q *queries) WasEventProcessed(ctx context.Context, eventID string) (bool, error) {
	var exists bool
	err := q.db.QueryRow(ctx, wasEventProcessed, eventID).Scan(&exists)
	return exists, err
}

const markEventProcessed = `INSERT INTO processed_events (event_id) VALUES($1) ON CONFLICT DO NOTHING`

func (q *queries) MarkEventProcessed(ctx context.Context, eventID string) error {
	_, err := q.db.Exec(ctx, markEventProcessed, eventID)
	return err
}

// ── Scan helpers ──────────────────────────────────────────────────────────────

func scanTask(r pgx.Row) (*Task, error) {
	t := &Task{}
	err := r.Scan(&t.ID, &t.BoardID, &t.ProjectID, &t.ColumnID,
		&t.Title, &t.Description, &t.Status, &t.Priority,
		&t.AssigneeID, &t.DueDate, &t.StartDate, &t.Labels, &t.Position,
		&t.CreatedBy, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return t, err
}

func scanTaskRow(r pgx.Rows) (*Task, error) {
	t := &Task{}
	err := r.Scan(&t.ID, &t.BoardID, &t.ProjectID, &t.ColumnID,
		&t.Title, &t.Description, &t.Status, &t.Priority,
		&t.AssigneeID, &t.DueDate, &t.StartDate, &t.Labels, &t.Position,
		&t.CreatedBy, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt)
	return t, err
}

func scanComment(r pgx.Row) (*Comment, error) {
	c := &Comment{}
	err := r.Scan(&c.ID, &c.TaskID, &c.AuthorID, &c.Body, &c.EditedAt, &c.CreatedAt, &c.DeletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return c, err
}

func scanCommentRow(r pgx.Rows) (*Comment, error) {
	c := &Comment{}
	err := r.Scan(&c.ID, &c.TaskID, &c.AuthorID, &c.Body, &c.EditedAt, &c.CreatedAt, &c.DeletedAt)
	return c, err
}

func scanAttachment(r pgx.Row) (*Attachment, error) {
	a := &Attachment{}
	err := r.Scan(&a.ID, &a.TaskID, &a.DocumentID, &a.AddedBy, &a.AddedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return a, err
}

func scanAttachmentRow(r pgx.Rows) (*Attachment, error) {
	a := &Attachment{}
	err := r.Scan(&a.ID, &a.TaskID, &a.DocumentID, &a.AddedBy, &a.AddedAt)
	return a, err
}

func scanIDProjectPairs(rows pgx.Rows, err error) ([]struct{ ID, ProjectID uuid.UUID }, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct{ ID, ProjectID uuid.UUID }
	for rows.Next() {
		var r struct{ ID, ProjectID uuid.UUID }
		if err := rows.Scan(&r.ID, &r.ProjectID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// unmarshalJSON is used by the repository to decode JSONB diff fields.
func unmarshalJSON(data []byte, v any) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}
