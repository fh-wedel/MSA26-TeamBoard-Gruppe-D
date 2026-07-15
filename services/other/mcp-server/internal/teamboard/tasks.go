package teamboard

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// maxTasksPerListCall bounds how many tasks ListTasks will fetch across pages,
// so a single tool call can't runaway-paginate a huge board.
const maxTasksPerListCall = 500

// ListTasks returns tasks on a board, optionally filtered by status and/or
// column, paginating internally (the caller never sees cursors).
func (c *Client) ListTasks(ctx context.Context, boardID, status, columnID string) ([]Task, error) {
	var all []Task
	cursor := ""
	for {
		q := url.Values{}
		if status != "" {
			q.Set("status", status)
		}
		if columnID != "" {
			q.Set("column_id", columnID)
		}
		if cursor != "" {
			q.Set("cursor", cursor)
		}

		path := fmt.Sprintf("/boards/%s/tasks", boardID)
		if enc := q.Encode(); enc != "" {
			path += "?" + enc
		}

		var page []Task
		next, err := c.doList(ctx, "GET", path, &page)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)

		if next == "" || len(all) >= maxTasksPerListCall || len(page) == 0 {
			break
		}
		cursor = next
	}
	return all, nil
}

// GetTask returns full detail for a single task.
func (c *Client) GetTask(ctx context.Context, taskID string) (*Task, error) {
	var task Task
	if err := c.doItem(ctx, "GET", fmt.Sprintf("/tasks/%s", taskID), nil, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// CreateTaskInput mirrors services/task/internal/api/dto.go's createTaskRequest.
type CreateTaskInput struct {
	ColumnID    *string    `json:"column_id,omitempty"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Priority    string     `json:"priority,omitempty"`
	AssigneeID  *string    `json:"assignee_id,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	Labels      []string   `json:"labels,omitempty"`
}

// CreateTask creates a task on a board. Priority defaults to "medium" and the
// task's status is derived from the target column's semantic status server-side.
func (c *Client) CreateTask(ctx context.Context, boardID string, in CreateTaskInput) (*Task, error) {
	var task Task
	if err := c.doItem(ctx, "POST", fmt.Sprintf("/boards/%s/tasks", boardID), in, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// UpdateTask applies a merge patch to a task (same semantics as the web UI's
// PATCH /tasks/{id}: only present keys change; an explicit null clears a date).
func (c *Client) UpdateTask(ctx context.Context, taskID string, patch map[string]any) (*Task, error) {
	var task Task
	if err := c.doItem(ctx, "PATCH", fmt.Sprintf("/tasks/%s", taskID), patch, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// MoveTask moves a task into a column (the kanban drag). The Task Service
// re-derives the task's status from the destination column. beforeID/afterID
// optionally pin the position between two existing tasks.
func (c *Client) MoveTask(ctx context.Context, taskID, columnID string, beforeID, afterID *string) (*Task, error) {
	body := map[string]any{"column_id": columnID}
	if beforeID != nil {
		body["before_id"] = *beforeID
	}
	if afterID != nil {
		body["after_id"] = *afterID
	}
	var task Task
	if err := c.doItem(ctx, "POST", fmt.Sprintf("/tasks/%s/move", taskID), body, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// AssignTask sets or clears (nil) a task's assignee.
func (c *Client) AssignTask(ctx context.Context, taskID string, assigneeID *string) (*Task, error) {
	var task Task
	if err := c.doItem(ctx, "POST", fmt.Sprintf("/tasks/%s/assign", taskID), map[string]any{"assignee_id": assigneeID}, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// DeleteTask permanently deletes a task.
func (c *Client) DeleteTask(ctx context.Context, taskID string) error {
	return c.doItem(ctx, "DELETE", fmt.Sprintf("/tasks/%s", taskID), nil, nil)
}

// ListComments returns a task's comments.
func (c *Client) ListComments(ctx context.Context, taskID string) ([]Comment, error) {
	var comments []Comment
	if _, err := c.doList(ctx, "GET", fmt.Sprintf("/tasks/%s/comments", taskID), &comments); err != nil {
		return nil, err
	}
	return comments, nil
}

// CreateComment adds a comment to a task.
func (c *Client) CreateComment(ctx context.Context, taskID, body string) (*Comment, error) {
	var comment Comment
	if err := c.doItem(ctx, "POST", fmt.Sprintf("/tasks/%s/comments", taskID), map[string]string{"body": body}, &comment); err != nil {
		return nil, err
	}
	return &comment, nil
}
