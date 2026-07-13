package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/teamboard/services/task/internal/ordering"
)

type service struct {
	repo   Repository
	projCl ProjectClient
	docCl  DocumentClient
}

// NewTaskService constructs the domain service.
func NewTaskService(repo Repository, projCl ProjectClient, docCl DocumentClient) TaskService {
	return &service{repo: repo, projCl: projCl, docCl: docCl}
}

var _ TaskService = (*service)(nil)

// ── Permission helper ─────────────────────────────────────────────────────────

func (s *service) requirePermission(ctx context.Context, projectID, userID uuid.UUID, perm string) error {
	ps, err := s.projCl.GetPermissions(ctx, projectID, userID)
	if err != nil {
		return fmt.Errorf("permission check: %w", err)
	}
	if !ps.IsMember || !ps.Has(perm) {
		return ErrPermissionDenied
	}
	return nil
}

// ── Tasks ─────────────────────────────────────────────────────────────────────

func (s *service) CreateTask(ctx context.Context, requester uuid.UUID, input CreateTaskInput) (*Task, error) {
	if input.Title == "" || len(input.Title) > 500 {
		return nil, ErrValidation
	}
	if input.Priority == "" {
		input.Priority = PriorityMedium
	}
	if !ValidPriority(input.Priority) {
		return nil, ErrValidation
	}
	if len(input.Labels) == 0 {
		input.Labels = []string{}
	}

	board, err := s.repo.GetKnownBoard(ctx, input.BoardID)
	if err != nil {
		return nil, ErrBoardUnknown
	}

	if err := s.requirePermission(ctx, board.ProjectID, requester, "task:create"); err != nil {
		return nil, err
	}

	// Validate column belongs to board (skip for null column_id on calendar boards).
	if input.ColumnID != nil {
		ok, err := s.repo.ColumnBelongsToBoard(ctx, *input.ColumnID, input.BoardID)
		if err != nil || !ok {
			return nil, ErrColumnNotInBoard
		}
	}

	// Compute position (append to end of column).
	lastPos, err := s.repo.GetLastPositionInColumn(ctx, input.BoardID, input.ColumnID)
	if err != nil {
		return nil, fmt.Errorf("get last position: %w", err)
	}
	pos, err := ordering.Between(lastPos, "")
	if err != nil {
		return nil, ErrInvalidPosition
	}

	// Use the column's explicit status (falling back to name derivation).
	status := StatusOpen
	if input.ColumnID != nil {
		col, err := s.repo.GetKnownColumn(ctx, *input.ColumnID)
		if err == nil {
			status = statusForColumn(col)
		}
	}

	var task *Task
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		task, txErr = tx.CreateTask(ctx,
			uuid.New(), input.BoardID, board.ProjectID, input.ColumnID,
			input.Title, input.Description, status, input.Priority,
			input.AssigneeID, input.DueDate, input.StartDate, input.Labels, pos, requester)
		if txErr != nil {
			return txErr
		}

		diff := map[string]any{"title": input.Title, "status": status, "priority": input.Priority}
		if txErr = tx.CreateHistoryEntry(ctx, uuid.New(), task.ID, requester, ChangeCreated, diff); txErr != nil {
			return txErr
		}

		payload, _ := json.Marshal(map[string]any{
			"task_id": task.ID, "board_id": input.BoardID, "project_id": board.ProjectID,
			"column_id": input.ColumnID, "title": task.Title,
			"assignee_id": task.AssigneeID, "priority": task.Priority,
			"labels": task.Labels, "created_by": requester,
		})
		return tx.InsertOutboxEvent(ctx, uuid.New(), task.ID, "task.created", payload)
	})
	if err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "task created", "task_id", task.ID, "board_id", input.BoardID)
	return task, nil
}

func (s *service) GetTask(ctx context.Context, taskID, requester uuid.UUID) (*Task, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, ErrTaskNotFound
	}
	if err := s.requirePermission(ctx, task.ProjectID, requester, "task:read"); err != nil {
		return nil, err
	}
	return s.repo.GetTaskWithCounts(ctx, taskID)
}

func (s *service) ListTasks(ctx context.Context, boardID, requester uuid.UUID, filter TaskFilter, page Pagination) ([]*Task, *PageCursor, error) {
	board, err := s.repo.GetKnownBoard(ctx, boardID)
	if err != nil {
		// Board not yet registered (event propagation delay) — no tasks can exist yet.
		return []*Task{}, nil, nil
	}
	if err := s.requirePermission(ctx, board.ProjectID, requester, "task:read"); err != nil {
		return nil, nil, err
	}

	limit := page.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	tasks, err := s.repo.ListTasksByBoard(ctx, boardID, filter, page.Cursor, limit+1)
	if err != nil {
		return nil, nil, err
	}

	var nextCursor *PageCursor
	if len(tasks) > limit {
		last := tasks[limit-1]
		nextCursor = &PageCursor{Position: last.Position, ID: last.ID}
		tasks = tasks[:limit]
	}
	return tasks, nextCursor, nil
}

func (s *service) UpdateTask(ctx context.Context, taskID, requester uuid.UUID, patch TaskPatch) (*Task, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, ErrTaskNotFound
	}
	if err := s.requirePermission(ctx, task.ProjectID, requester, "task:update"); err != nil {
		return nil, err
	}
	if patch.Priority != nil && !ValidPriority(*patch.Priority) {
		return nil, ErrValidation
	}
	if patch.Status != nil && !ValidStatus(*patch.Status) {
		return nil, ErrValidation
	}

	updated, err := s.repo.UpdateTask(ctx, taskID, patch)
	if err != nil {
		return nil, ErrTaskNotFound
	}

	statusChanged := patch.Status != nil && *patch.Status != task.Status

	diff := buildUpdateDiff(task, patch)
	var txErr error
	txErr = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		if txErr = tx.CreateHistoryEntry(ctx, uuid.New(), taskID, requester, ChangeUpdated, diff); txErr != nil {
			return txErr
		}
		payload, _ := json.Marshal(map[string]any{"task_id": taskID, "board_id": task.BoardID, "project_id": task.ProjectID, "changes": diff})
		if txErr = tx.InsertOutboxEvent(ctx, uuid.New(), taskID, "task.updated", payload); txErr != nil {
			return txErr
		}
		// Emit a dedicated status-change event so notification/live-update consumers
		// react identically whether the status changed via a move or a direct edit.
		if statusChanged {
			statusPayload, _ := json.Marshal(map[string]any{
				"task_id": taskID, "board_id": task.BoardID, "project_id": task.ProjectID,
				"from": task.Status, "to": *patch.Status,
			})
			return tx.InsertOutboxEvent(ctx, uuid.New(), taskID, "task.status.changed", statusPayload)
		}
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}

	return updated, nil
}

func (s *service) MoveTask(ctx context.Context, taskID, requester uuid.UUID, columnID uuid.UUID, beforeID, afterID *uuid.UUID) (*Task, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, ErrTaskNotFound
	}
	if err := s.requirePermission(ctx, task.ProjectID, requester, "task:update"); err != nil {
		return nil, err
	}

	// Validate column belongs to board.
	ok, err := s.repo.ColumnBelongsToBoard(ctx, columnID, task.BoardID)
	if err != nil || !ok {
		return nil, ErrColumnNotInBoard
	}

	// Compute new position.
	afterPos, beforePos, err := s.repo.GetPositionForRefs(ctx, afterID, beforeID)
	if err != nil {
		return nil, fmt.Errorf("get positions: %w", err)
	}
	newPos, err := ordering.Between(afterPos, beforePos)
	if err != nil {
		return nil, ErrInvalidPosition
	}

	// Use the destination column's explicit status (falling back to name derivation).
	newStatus := task.Status
	col, err := s.repo.GetKnownColumn(ctx, columnID)
	if err == nil {
		newStatus = statusForColumn(col)
	}

	oldColumnID := task.ColumnID
	oldStatus := task.Status

	var moved *Task
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		moved, txErr = tx.MoveTask(ctx, taskID, columnID, newPos, newStatus)
		if txErr != nil {
			return txErr
		}

		diff := map[string]any{
			"column_id": map[string]any{"from": oldColumnID, "to": columnID},
			"position":  map[string]any{"from": task.Position, "to": newPos},
		}
		if txErr = tx.CreateHistoryEntry(ctx, uuid.New(), taskID, requester, ChangeMoved, diff); txErr != nil {
			return txErr
		}

		movedPayload, _ := json.Marshal(map[string]any{
			"task_id": taskID, "board_id": task.BoardID, "project_id": task.ProjectID,
			"from_column_id": oldColumnID, "to_column_id": columnID, "position": newPos,
		})
		if txErr = tx.InsertOutboxEvent(ctx, uuid.New(), taskID, "task.moved", movedPayload); txErr != nil {
			return txErr
		}

		if newStatus != oldStatus {
			statusPayload, _ := json.Marshal(map[string]any{
				"task_id": taskID, "board_id": task.BoardID, "project_id": task.ProjectID,
				"from": oldStatus, "to": newStatus,
			})
			return tx.InsertOutboxEvent(ctx, uuid.New(), taskID, "task.status.changed", statusPayload)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return moved, nil
}

func (s *service) AssignTask(ctx context.Context, taskID, requester uuid.UUID, assigneeID *uuid.UUID) (*Task, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, ErrTaskNotFound
	}
	if err := s.requirePermission(ctx, task.ProjectID, requester, "task:update"); err != nil {
		return nil, err
	}

	if assigneeID != nil {
		perms, err := s.projCl.GetPermissions(ctx, task.ProjectID, *assigneeID)
		if err != nil || !perms.IsMember {
			return nil, ErrAssigneeNotMember
		}
	}

	prevAssignee := task.AssigneeID
	var updated *Task
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		updated, txErr = tx.AssignTask(ctx, taskID, assigneeID)
		if txErr != nil {
			return txErr
		}

		changeType := ChangeAssigned
		eventType := "task.assigned"
		if assigneeID == nil {
			changeType = ChangeUnassigned
			eventType = "task.unassigned"
		}
		diff := map[string]any{"assignee_id": map[string]any{"from": prevAssignee, "to": assigneeID}}
		if txErr = tx.CreateHistoryEntry(ctx, uuid.New(), taskID, requester, changeType, diff); txErr != nil {
			return txErr
		}
		payload, _ := json.Marshal(map[string]any{
			"task_id": taskID, "project_id": task.ProjectID,
			"assignee_id": assigneeID, "previous_assignee_id": prevAssignee, "assigned_by": requester,
		})
		return tx.InsertOutboxEvent(ctx, uuid.New(), taskID, eventType, payload)
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *service) DeleteTask(ctx context.Context, taskID, requester uuid.UUID) error {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return ErrTaskNotFound
	}
	if err := s.requirePermission(ctx, task.ProjectID, requester, "task:delete"); err != nil {
		return err
	}

	return s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		if err := tx.SoftDeleteTask(ctx, taskID); err != nil {
			return err
		}
		if err := tx.CreateHistoryEntry(ctx, uuid.New(), taskID, requester, ChangeDeleted, map[string]any{}); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{
			"task_id": taskID, "board_id": task.BoardID, "project_id": task.ProjectID,
			"assignee_id": task.AssigneeID, "deleted_by": requester,
		})
		return tx.InsertOutboxEvent(ctx, uuid.New(), taskID, "task.deleted", payload)
	})
}

// ── Comments ──────────────────────────────────────────────────────────────────

func (s *service) CreateComment(ctx context.Context, taskID, requester uuid.UUID, body string) (*Comment, error) {
	if body == "" || len(body) > 10000 {
		return nil, ErrValidation
	}

	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, ErrTaskNotFound
	}
	if err := s.requirePermission(ctx, task.ProjectID, requester, "comment:create"); err != nil {
		return nil, err
	}

	// Resolve @mentions to user IDs.
	handles := ExtractMentionHandles(body)
	mentions := make([]uuid.UUID, 0, len(handles))
	for _, h := range handles {
		// Try to resolve handle as user ID or email.
		if id, err := uuid.Parse(h); err == nil {
			if ok, _ := s.repo.KnownUserExists(ctx, id); ok {
				mentions = append(mentions, id)
			}
		}
		// Email-based resolution is not implemented here (no email lookup in task repo).
	}

	var comment *Comment
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		comment, txErr = tx.CreateComment(ctx, uuid.New(), taskID, requester, body)
		if txErr != nil {
			return txErr
		}
		for _, uid := range mentions {
			_ = tx.AddMention(ctx, comment.ID, uid)
		}
		comment.Mentions = mentions

		if txErr = tx.CreateHistoryEntry(ctx, uuid.New(), taskID, requester, ChangeCommented, map[string]any{"comment_id": comment.ID}); txErr != nil {
			return txErr
		}
		excerpt := body
		if len(excerpt) > 200 {
			excerpt = excerpt[:200]
		}
		payload, _ := json.Marshal(map[string]any{
			"task_id": taskID, "project_id": task.ProjectID,
			"comment_id": comment.ID, "author_id": requester,
			"body_excerpt": excerpt, "mentions": mentions,
		})
		return tx.InsertOutboxEvent(ctx, uuid.New(), taskID, "task.commented", payload)
	})
	if err != nil {
		return nil, err
	}
	return comment, nil
}

func (s *service) ListComments(ctx context.Context, taskID, requester uuid.UUID) ([]*Comment, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, ErrTaskNotFound
	}
	if err := s.requirePermission(ctx, task.ProjectID, requester, "comment:read"); err != nil {
		return nil, err
	}
	comments, err := s.repo.ListCommentsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	// Attach mentions.
	for _, c := range comments {
		c.Mentions, _ = s.repo.ListMentionsByComment(ctx, c.ID)
	}
	return comments, nil
}

func (s *service) UpdateComment(ctx context.Context, commentID, requester uuid.UUID, body string) (*Comment, error) {
	if body == "" || len(body) > 10000 {
		return nil, ErrValidation
	}

	comment, err := s.repo.GetComment(ctx, commentID)
	if err != nil {
		return nil, ErrCommentNotFound
	}
	if comment.AuthorID != requester {
		// Allow `task:delete` holders to edit too — but require a task read to get the project ID.
		task, terr := s.repo.GetTask(ctx, comment.TaskID)
		if terr != nil {
			return nil, ErrPermissionDenied
		}
		if perr := s.requirePermission(ctx, task.ProjectID, requester, "task:delete"); perr != nil {
			return nil, ErrNotCommentAuthor
		}
	}

	var updated *Comment
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		if err := tx.ArchiveCommentBody(ctx, uuid.New(), commentID, comment.Body); err != nil {
			return err
		}
		// Clear old mentions, extract new ones.
		_ = tx.ClearMentions(ctx, commentID)

		var txErr error
		updated, txErr = tx.UpdateComment(ctx, commentID, body)
		if txErr != nil {
			return txErr
		}

		// Re-resolve mentions.
		handles := ExtractMentionHandles(body)
		for _, h := range handles {
			if id, err := uuid.Parse(h); err == nil {
				if ok, _ := tx.KnownUserExists(ctx, id); ok {
					_ = tx.AddMention(ctx, commentID, id)
				}
			}
		}

		task, _ := tx.GetTask(ctx, comment.TaskID)
		if task != nil {
			payload, _ := json.Marshal(map[string]any{
				"task_id": comment.TaskID, "project_id": task.ProjectID, "comment_id": commentID,
			})
			return tx.InsertOutboxEvent(ctx, uuid.New(), commentID, "task.comment.updated", payload)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	updated.Mentions, _ = s.repo.ListMentionsByComment(ctx, commentID)
	return updated, nil
}

func (s *service) DeleteComment(ctx context.Context, commentID, requester uuid.UUID) error {
	comment, err := s.repo.GetComment(ctx, commentID)
	if err != nil {
		return ErrCommentNotFound
	}

	if comment.AuthorID != requester {
		task, terr := s.repo.GetTask(ctx, comment.TaskID)
		if terr != nil {
			return ErrPermissionDenied
		}
		if perr := s.requirePermission(ctx, task.ProjectID, requester, "task:delete"); perr != nil {
			return ErrNotCommentAuthor
		}
	}

	return s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		if err := tx.SoftDeleteComment(ctx, commentID); err != nil {
			return err
		}
		task, _ := tx.GetTask(ctx, comment.TaskID)
		if task != nil {
			payload, _ := json.Marshal(map[string]any{
				"task_id": comment.TaskID, "project_id": task.ProjectID, "comment_id": commentID,
			})
			return tx.InsertOutboxEvent(ctx, uuid.New(), commentID, "task.comment.deleted", payload)
		}
		return nil
	})
}

// ── Attachments ───────────────────────────────────────────────────────────────

func (s *service) AddAttachment(ctx context.Context, taskID, requester, documentID uuid.UUID) (*Attachment, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, ErrTaskNotFound
	}
	if err := s.requirePermission(ctx, task.ProjectID, requester, "task:update"); err != nil {
		return nil, err
	}

	docInfo, err := s.docCl.GetDocumentInfo(ctx, documentID)
	if err != nil {
		return nil, ErrDocumentUnreachable
	}
	if docInfo.ProjectID != task.ProjectID {
		return nil, ErrDocumentNotInProject
	}

	var att *Attachment
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		att, txErr = tx.CreateAttachment(ctx, uuid.New(), taskID, documentID, requester)
		if txErr != nil {
			return txErr
		}
		if txErr = tx.CreateHistoryEntry(ctx, uuid.New(), taskID, requester, ChangeAttachmentAdded, map[string]any{"document_id": documentID}); txErr != nil {
			return txErr
		}
		payload, _ := json.Marshal(map[string]any{
			"task_id": taskID, "project_id": task.ProjectID,
			"attachment_id": att.ID, "document_id": documentID, "added_by": requester,
		})
		return tx.InsertOutboxEvent(ctx, uuid.New(), att.ID, "task.attachment.added", payload)
	})
	if err != nil {
		return nil, err
	}
	return att, nil
}

func (s *service) ListAttachments(ctx context.Context, taskID, requester uuid.UUID) ([]*Attachment, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, ErrTaskNotFound
	}
	if err := s.requirePermission(ctx, task.ProjectID, requester, "task:read"); err != nil {
		return nil, err
	}
	return s.repo.ListAttachmentsByTask(ctx, taskID)
}

func (s *service) RemoveAttachment(ctx context.Context, attachmentID, requester uuid.UUID) error {
	att, err := s.repo.GetAttachment(ctx, attachmentID)
	if err != nil {
		return ErrAttachmentNotFound
	}
	task, err := s.repo.GetTask(ctx, att.TaskID)
	if err != nil {
		return ErrTaskNotFound
	}
	if err := s.requirePermission(ctx, task.ProjectID, requester, "task:update"); err != nil {
		return err
	}

	return s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		if err := tx.DeleteAttachment(ctx, attachmentID); err != nil {
			return err
		}
		if err := tx.CreateHistoryEntry(ctx, uuid.New(), att.TaskID, requester, ChangeAttachmentRemoved, map[string]any{"document_id": att.DocumentID}); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{
			"task_id": att.TaskID, "project_id": task.ProjectID,
			"attachment_id": attachmentID, "document_id": att.DocumentID,
		})
		return tx.InsertOutboxEvent(ctx, uuid.New(), attachmentID, "task.attachment.removed", payload)
	})
}

// ── History ───────────────────────────────────────────────────────────────────

func (s *service) GetTaskHistory(ctx context.Context, taskID, requester uuid.UUID) ([]*HistoryEntry, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, ErrTaskNotFound
	}
	if err := s.requirePermission(ctx, task.ProjectID, requester, "task:read"); err != nil {
		return nil, err
	}
	return s.repo.ListTaskHistory(ctx, taskID, 100)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func buildUpdateDiff(old *Task, patch TaskPatch) map[string]any {
	diff := map[string]any{}
	if patch.Title != nil && *patch.Title != old.Title {
		diff["title"] = map[string]any{"from": old.Title, "to": *patch.Title}
	}
	if patch.Description != nil && *patch.Description != old.Description {
		diff["description"] = map[string]any{"from": old.Description, "to": *patch.Description}
	}
	if patch.Priority != nil && *patch.Priority != old.Priority {
		diff["priority"] = map[string]any{"from": old.Priority, "to": *patch.Priority}
	}
	if patch.Status != nil && *patch.Status != old.Status {
		diff["status"] = map[string]any{"from": old.Status, "to": *patch.Status}
	}
	if patch.DueDateSet {
		diff["due_date"] = map[string]any{"from": old.DueDate, "to": patch.DueDate}
	}
	if patch.StartDateSet {
		diff["start_date"] = map[string]any{"from": old.StartDate, "to": patch.StartDate}
	}
	if patch.Labels != nil {
		diff["labels"] = map[string]any{"from": old.Labels, "to": *patch.Labels}
	}
	return diff
}
