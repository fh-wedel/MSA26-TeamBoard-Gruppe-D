package teamboard

import (
	"context"
	"fmt"
	"net/url"
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
