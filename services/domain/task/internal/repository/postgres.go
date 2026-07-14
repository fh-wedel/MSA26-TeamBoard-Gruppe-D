package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamboard/services/domain/task/internal/domain"
	"github.com/teamboard/services/domain/task/internal/repository/db"
)

type postgresRepo struct {
	pool *pgxpool.Pool
	q    db.Querier
}

func New(pool *pgxpool.Pool) domain.Repository {
	return &postgresRepo{pool: pool, q: db.New(pool)}
}

func (r *postgresRepo) WithTransaction(ctx context.Context, fn func(context.Context, domain.Repository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	txRepo := &postgresRepo{pool: r.pool, q: db.New(tx)}
	if err := fn(ctx, txRepo); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// ── Tasks ─────────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateTask(ctx context.Context, id, boardID, projectID uuid.UUID, columnID *uuid.UUID, title, description string, status domain.Status, priority domain.Priority, assigneeID *uuid.UUID, dueDate, startDate *time.Time, labels []string, position string, createdBy uuid.UUID) (*domain.Task, error) {
	t, err := r.q.CreateTask(ctx, id, boardID, projectID, columnID, title, description, string(status), string(priority), assigneeID, dueDate, startDate, labels, position, createdBy)
	if err != nil {
		return nil, err
	}
	return mapTask(t), nil
}

func (r *postgresRepo) GetTask(ctx context.Context, id uuid.UUID) (*domain.Task, error) {
	t, err := r.q.GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTaskNotFound
		}
		return nil, err
	}
	return mapTask(t), nil
}

func (r *postgresRepo) GetTaskWithCounts(ctx context.Context, id uuid.UUID) (*domain.Task, error) {
	t, err := r.q.GetTaskWithCounts(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTaskNotFound
		}
		return nil, err
	}
	task := mapTask(&t.Task)
	task.CommentCount = t.CommentCount
	task.AttachmentCount = t.AttachmentCount
	return task, nil
}

func (r *postgresRepo) ListTasksByBoard(ctx context.Context, boardID uuid.UUID, filter domain.TaskFilter, cursor *string, limit int) ([]*domain.Task, error) {
	var statusStr *string
	if filter.Status != nil {
		s := string(*filter.Status)
		statusStr = &s
	}
	tasks, err := r.q.ListTasksByBoard(ctx, boardID, statusStr, filter.AssigneeID, filter.ColumnID, filter.Label, cursor, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Task, len(tasks))
	for i, t := range tasks {
		out[i] = mapTask(t)
	}
	return out, nil
}

func (r *postgresRepo) GetLastPositionInColumn(ctx context.Context, boardID uuid.UUID, columnID *uuid.UUID) (string, error) {
	return r.q.GetLastPositionInColumn(ctx, boardID, columnID)
}

func (r *postgresRepo) GetPositionForRefs(ctx context.Context, afterID, beforeID *uuid.UUID) (string, string, error) {
	ids := make([]uuid.UUID, 0, 2)
	if afterID != nil {
		ids = append(ids, *afterID)
	}
	if beforeID != nil {
		ids = append(ids, *beforeID)
	}
	if len(ids) == 0 {
		return "", "", nil
	}
	posMap, err := r.q.GetPositionsByIDs(ctx, ids)
	if err != nil {
		return "", "", err
	}
	afterPos := ""
	beforePos := ""
	if afterID != nil {
		afterPos = posMap[*afterID]
	}
	if beforeID != nil {
		beforePos = posMap[*beforeID]
	}
	return afterPos, beforePos, nil
}

func (r *postgresRepo) UpdateTask(ctx context.Context, id uuid.UUID, patch domain.TaskPatch) (*domain.Task, error) {
	var priority *string
	if patch.Priority != nil {
		s := string(*patch.Priority)
		priority = &s
	}
	var labelsArg []string
	if patch.Labels != nil {
		labelsArg = *patch.Labels
	}
	var status *string
	if patch.Status != nil {
		s := string(*patch.Status)
		status = &s
	}
	t, err := r.q.UpdateTask(ctx, id, patch.Title, patch.Description, priority, patch.DueDateSet, patch.DueDate, patch.StartDateSet, patch.StartDate, labelsArg, status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTaskNotFound
		}
		return nil, err
	}
	return mapTask(t), nil
}

func (r *postgresRepo) MoveTask(ctx context.Context, id, columnID uuid.UUID, position string, status domain.Status) (*domain.Task, error) {
	t, err := r.q.MoveTask(ctx, id, columnID, position, string(status))
	if err != nil {
		return nil, err
	}
	return mapTask(t), nil
}

func (r *postgresRepo) AssignTask(ctx context.Context, id uuid.UUID, assigneeID *uuid.UUID) (*domain.Task, error) {
	t, err := r.q.AssignTask(ctx, id, assigneeID)
	if err != nil {
		return nil, err
	}
	return mapTask(t), nil
}

func (r *postgresRepo) SoftDeleteTask(ctx context.Context, id uuid.UUID) error {
	return r.q.SoftDeleteTask(ctx, id)
}

func (r *postgresRepo) SoftDeleteTasksByProject(ctx context.Context, projectID uuid.UUID) ([]uuid.UUID, error) {
	pairs, err := r.q.SoftDeleteTasksByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(pairs))
	for i, p := range pairs {
		ids[i] = p.ID
	}
	return ids, nil
}

func (r *postgresRepo) SoftDeleteTasksByBoard(ctx context.Context, boardID uuid.UUID) ([]uuid.UUID, error) {
	pairs, err := r.q.SoftDeleteTasksByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(pairs))
	for i, p := range pairs {
		ids[i] = p.ID
	}
	return ids, nil
}

func (r *postgresRepo) ClearAssigneeForUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	pairs, err := r.q.ClearAssigneeForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(pairs))
	for i, p := range pairs {
		ids[i] = p.ID
	}
	return ids, nil
}

func (r *postgresRepo) NullifyColumnReferences(ctx context.Context, columnID uuid.UUID) error {
	return r.q.NullifyColumnReferences(ctx, columnID)
}

// ── Comments ──────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateComment(ctx context.Context, id, taskID, authorID uuid.UUID, body string) (*domain.Comment, error) {
	c, err := r.q.CreateComment(ctx, id, taskID, authorID, body)
	if err != nil {
		return nil, err
	}
	return mapComment(c), nil
}

func (r *postgresRepo) GetComment(ctx context.Context, id uuid.UUID) (*domain.Comment, error) {
	c, err := r.q.GetComment(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrCommentNotFound
		}
		return nil, err
	}
	return mapComment(c), nil
}

func (r *postgresRepo) ListCommentsByTask(ctx context.Context, taskID uuid.UUID) ([]*domain.Comment, error) {
	rows, err := r.q.ListCommentsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Comment, len(rows))
	for i, c := range rows {
		out[i] = mapComment(c)
	}
	return out, nil
}

func (r *postgresRepo) UpdateComment(ctx context.Context, id uuid.UUID, body string) (*domain.Comment, error) {
	c, err := r.q.UpdateComment(ctx, id, body)
	if err != nil {
		return nil, err
	}
	return mapComment(c), nil
}

func (r *postgresRepo) SoftDeleteComment(ctx context.Context, id uuid.UUID) error {
	return r.q.SoftDeleteComment(ctx, id)
}

func (r *postgresRepo) ArchiveCommentBody(ctx context.Context, historyID, commentID uuid.UUID, body string) error {
	return r.q.ArchiveCommentBody(ctx, historyID, commentID, body)
}

func (r *postgresRepo) AddMention(ctx context.Context, commentID, userID uuid.UUID) error {
	return r.q.AddMention(ctx, commentID, userID)
}

func (r *postgresRepo) ListMentionsByComment(ctx context.Context, commentID uuid.UUID) ([]uuid.UUID, error) {
	return r.q.ListMentionsByComment(ctx, commentID)
}

func (r *postgresRepo) ClearMentions(ctx context.Context, commentID uuid.UUID) error {
	return r.q.ClearMentions(ctx, commentID)
}

// ── Attachments ───────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateAttachment(ctx context.Context, id, taskID, documentID, addedBy uuid.UUID) (*domain.Attachment, error) {
	a, err := r.q.CreateAttachment(ctx, id, taskID, documentID, addedBy)
	if err != nil {
		return nil, err
	}
	return mapAttachment(a), nil
}

func (r *postgresRepo) GetAttachment(ctx context.Context, id uuid.UUID) (*domain.Attachment, error) {
	a, err := r.q.GetAttachment(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrAttachmentNotFound
		}
		return nil, err
	}
	return mapAttachment(a), nil
}

func (r *postgresRepo) ListAttachmentsByTask(ctx context.Context, taskID uuid.UUID) ([]*domain.Attachment, error) {
	rows, err := r.q.ListAttachmentsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Attachment, len(rows))
	for i, a := range rows {
		out[i] = mapAttachment(a)
	}
	return out, nil
}

func (r *postgresRepo) DeleteAttachment(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteAttachment(ctx, id)
}

func (r *postgresRepo) DeleteAttachmentsByDocument(ctx context.Context, documentID uuid.UUID) ([]struct{ ID, TaskID uuid.UUID }, error) {
	return r.q.DeleteAttachmentsByDocument(ctx, documentID)
}

// ── History ───────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateHistoryEntry(ctx context.Context, id, taskID, actorID uuid.UUID, changeType domain.ChangeType, diff map[string]any) error {
	data, _ := json.Marshal(diff)
	return r.q.CreateHistoryEntry(ctx, id, taskID, actorID, string(changeType), data)
}

func (r *postgresRepo) ListTaskHistory(ctx context.Context, taskID uuid.UUID, limit int) ([]*domain.HistoryEntry, error) {
	rows, err := r.q.ListTaskHistory(ctx, taskID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.HistoryEntry, len(rows))
	for i, h := range rows {
		var diff map[string]any
		_ = json.Unmarshal(h.Diff, &diff)
		out[i] = &domain.HistoryEntry{
			ID:         h.ID,
			TaskID:     h.TaskID,
			ActorID:    h.ActorID,
			ChangeType: domain.ChangeType(h.ChangeType),
			Diff:       diff,
			OccurredAt: h.OccurredAt,
		}
	}
	return out, nil
}

// ── Known boards ──────────────────────────────────────────────────────────────

func (r *postgresRepo) UpsertKnownBoard(ctx context.Context, id, projectID uuid.UUID, name, bType string) error {
	return r.q.UpsertKnownBoard(ctx, id, projectID, name, bType)
}

func (r *postgresRepo) GetKnownBoard(ctx context.Context, id uuid.UUID) (*domain.KnownBoard, error) {
	b, err := r.q.GetKnownBoard(ctx, id)
	if err != nil {
		return nil, domain.ErrBoardUnknown
	}
	return &domain.KnownBoard{ID: b.ID, ProjectID: b.ProjectID, Name: b.Name, Type: b.Type, DeletedAt: b.DeletedAt}, nil
}

func (r *postgresRepo) MarkBoardDeleted(ctx context.Context, id uuid.UUID) error {
	return r.q.MarkBoardDeleted(ctx, id)
}

func (r *postgresRepo) UpsertKnownColumn(ctx context.Context, id, boardID uuid.UUID, name string, position int, status string) error {
	return r.q.UpsertKnownColumn(ctx, id, boardID, name, position, status)
}

func (r *postgresRepo) GetKnownColumn(ctx context.Context, id uuid.UUID) (*domain.KnownColumn, error) {
	c, err := r.q.GetKnownColumn(ctx, id)
	if err != nil {
		return nil, err
	}
	return &domain.KnownColumn{ID: c.ID, BoardID: c.BoardID, Name: c.Name, Position: c.Position, Status: c.Status}, nil
}

func (r *postgresRepo) ColumnBelongsToBoard(ctx context.Context, columnID, boardID uuid.UUID) (bool, error) {
	return r.q.ColumnBelongsToBoard(ctx, columnID, boardID)
}

func (r *postgresRepo) DeleteKnownColumn(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteKnownColumn(ctx, id)
}

func (r *postgresRepo) UpsertKnownUser(ctx context.Context, id uuid.UUID, email string) error {
	return r.q.UpsertKnownUser(ctx, id, email)
}

func (r *postgresRepo) KnownUserExists(ctx context.Context, id uuid.UUID) (bool, error) {
	return r.q.KnownUserExists(ctx, id)
}

func (r *postgresRepo) MarkKnownUserDeleted(ctx context.Context, id uuid.UUID) error {
	return r.q.MarkKnownUserDeleted(ctx, id)
}

// ── Outbox ────────────────────────────────────────────────────────────────────

func (r *postgresRepo) InsertOutboxEvent(ctx context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error {
	return r.q.InsertOutboxEvent(ctx, id, aggregateID, eventType, payload)
}

func (r *postgresRepo) GetUnpublishedEvents(ctx context.Context, limit int32) ([]*domain.OutboxEvent, error) {
	rows, err := r.q.GetUnpublishedEvents(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.OutboxEvent, len(rows))
	for i, e := range rows {
		out[i] = &domain.OutboxEvent{
			ID: e.ID, AggregateID: e.AggregateID, EventType: e.EventType,
			Payload: e.Payload, OccurredAt: e.OccurredAt, PublishedAt: e.PublishedAt,
		}
	}
	return out, nil
}

func (r *postgresRepo) MarkEventPublished(ctx context.Context, id uuid.UUID) error {
	return r.q.MarkEventPublished(ctx, id)
}

func (r *postgresRepo) WasEventProcessed(ctx context.Context, eventID string) (bool, error) {
	return r.q.WasEventProcessed(ctx, eventID)
}

func (r *postgresRepo) MarkEventProcessed(ctx context.Context, eventID string) error {
	return r.q.MarkEventProcessed(ctx, eventID)
}

// ── Mapping helpers ───────────────────────────────────────────────────────────

func mapTask(t *db.Task) *domain.Task {
	return &domain.Task{
		ID: t.ID, BoardID: t.BoardID, ProjectID: t.ProjectID, ColumnID: t.ColumnID,
		Title: t.Title, Description: t.Description,
		Status: domain.Status(t.Status), Priority: domain.Priority(t.Priority),
		AssigneeID: t.AssigneeID, DueDate: t.DueDate, StartDate: t.StartDate,
		Labels: t.Labels, Position: t.Position,
		CreatedBy: t.CreatedBy, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, DeletedAt: t.DeletedAt,
	}
}

func mapComment(c *db.Comment) *domain.Comment {
	return &domain.Comment{
		ID: c.ID, TaskID: c.TaskID, AuthorID: c.AuthorID,
		Body: c.Body, EditedAt: c.EditedAt, CreatedAt: c.CreatedAt, DeletedAt: c.DeletedAt,
	}
}

func mapAttachment(a *db.Attachment) *domain.Attachment {
	return &domain.Attachment{
		ID: a.ID, TaskID: a.TaskID, DocumentID: a.DocumentID, AddedBy: a.AddedBy, AddedAt: a.AddedAt,
	}
}
