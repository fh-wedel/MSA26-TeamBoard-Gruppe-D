package api

import (
	"time"

	"github.com/google/uuid"
	"github.com/teamboard/services/task/internal/domain"
)

// ── Request DTOs ──────────────────────────────────────────────────────────────

type createTaskRequest struct {
	ColumnID    *uuid.UUID `json:"column_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Priority    string     `json:"priority"`
	AssigneeID  *uuid.UUID `json:"assignee_id"`
	DueDate     *time.Time `json:"due_date"`
	Labels      []string   `json:"labels"`
}

type moveTaskRequest struct {
	ColumnID uuid.UUID  `json:"column_id"`
	BeforeID *uuid.UUID `json:"before_id"` // task that will come immediately before this one
	AfterID  *uuid.UUID `json:"after_id"`  // task that will come immediately after this one
}

type assignTaskRequest struct {
	AssigneeID *uuid.UUID `json:"assignee_id"`
}

type createCommentRequest struct {
	Body string `json:"body"`
}

type updateCommentRequest struct {
	Body string `json:"body"`
}

type addAttachmentRequest struct {
	DocumentID uuid.UUID `json:"document_id"`
}

// ── Response DTOs ─────────────────────────────────────────────────────────────

type taskResponse struct {
	ID              uuid.UUID  `json:"id"`
	BoardID         uuid.UUID  `json:"board_id"`
	ProjectID       uuid.UUID  `json:"project_id"`
	ColumnID        *uuid.UUID `json:"column_id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	Status          string     `json:"status"`
	Priority        string     `json:"priority"`
	AssigneeID      *uuid.UUID `json:"assignee_id"`
	DueDate         *time.Time `json:"due_date"`
	Labels          []string   `json:"labels"`
	Position        string     `json:"position"`
	CreatedBy       uuid.UUID  `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CommentCount    int        `json:"comment_count"`
	AttachmentCount int        `json:"attachment_count"`
}

type commentResponse struct {
	ID        uuid.UUID   `json:"id"`
	TaskID    uuid.UUID   `json:"task_id"`
	AuthorID  uuid.UUID   `json:"author_id"`
	Body      string      `json:"body"`
	Mentions  []uuid.UUID `json:"mentions"`
	EditedAt  *time.Time  `json:"edited_at"`
	CreatedAt time.Time   `json:"created_at"`
}

type attachmentResponse struct {
	ID         uuid.UUID `json:"id"`
	TaskID     uuid.UUID `json:"task_id"`
	DocumentID uuid.UUID `json:"document_id"`
	AddedBy    uuid.UUID `json:"added_by"`
	AddedAt    time.Time `json:"added_at"`
}

type historyEntryResponse struct {
	ID         uuid.UUID      `json:"id"`
	TaskID     uuid.UUID      `json:"task_id"`
	ActorID    uuid.UUID      `json:"actor_id"`
	ChangeType string         `json:"change_type"`
	Diff       map[string]any `json:"diff"`
	OccurredAt time.Time      `json:"occurred_at"`
}

type paginationResponse struct {
	NextCursor *string `json:"next_cursor"`
	Limit      int     `json:"limit"`
}

// ── Mapping functions ─────────────────────────────────────────────────────────

func mapTask(t *domain.Task) taskResponse {
	return taskResponse{
		ID:              t.ID,
		BoardID:         t.BoardID,
		ProjectID:       t.ProjectID,
		ColumnID:        t.ColumnID,
		Title:           t.Title,
		Description:     t.Description,
		Status:          string(t.Status),
		Priority:        string(t.Priority),
		AssigneeID:      t.AssigneeID,
		DueDate:         t.DueDate,
		Labels:          t.Labels,
		Position:        t.Position,
		CreatedBy:       t.CreatedBy,
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
		CommentCount:    t.CommentCount,
		AttachmentCount: t.AttachmentCount,
	}
}

func mapComment(c *domain.Comment) commentResponse {
	mentions := c.Mentions
	if mentions == nil {
		mentions = []uuid.UUID{}
	}
	return commentResponse{
		ID:        c.ID,
		TaskID:    c.TaskID,
		AuthorID:  c.AuthorID,
		Body:      c.Body,
		Mentions:  mentions,
		EditedAt:  c.EditedAt,
		CreatedAt: c.CreatedAt,
	}
}

func mapAttachment(a *domain.Attachment) attachmentResponse {
	return attachmentResponse{
		ID:         a.ID,
		TaskID:     a.TaskID,
		DocumentID: a.DocumentID,
		AddedBy:    a.AddedBy,
		AddedAt:    a.AddedAt,
	}
}

func mapHistoryEntry(e *domain.HistoryEntry) historyEntryResponse {
	diff := e.Diff
	if diff == nil {
		diff = map[string]any{}
	}
	return historyEntryResponse{
		ID:         e.ID,
		TaskID:     e.TaskID,
		ActorID:    e.ActorID,
		ChangeType: string(e.ChangeType),
		Diff:       diff,
		OccurredAt: e.OccurredAt,
	}
}
