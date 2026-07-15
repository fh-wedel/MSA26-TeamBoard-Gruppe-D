package teamboard

import (
	"context"
	"fmt"
)

// ListBoards returns all boards in a single project.
func (c *Client) ListBoards(ctx context.Context, projectID string) ([]Board, error) {
	var boards []Board
	if _, err := c.doList(ctx, "GET", fmt.Sprintf("/projects/%s/boards", projectID), &boards); err != nil {
		return nil, err
	}
	return boards, nil
}

// GetBoard returns a single board with its columns, mirroring the frontend's
// boardsApi.get (GET /boards/{id} — boards are not addressed via their project).
func (c *Client) GetBoard(ctx context.Context, boardID string) (*Board, error) {
	var board Board
	if err := c.doItem(ctx, "GET", fmt.Sprintf("/boards/%s", boardID), nil, &board); err != nil {
		return nil, err
	}
	return &board, nil
}

// CreateBoard creates a board in a project. Columns are intentionally not
// passed: the Project Service seeds the board type's default columns (with
// their semantic status) from the Board Registry. config overrides the type's
// default_config and is validated against its config_schema.
func (c *Client) CreateBoard(ctx context.Context, projectID, name, boardType string, config map[string]any) (*Board, error) {
	body := map[string]any{"name": name, "type": boardType}
	if config != nil {
		body["config"] = config
	}
	var board Board
	if err := c.doItem(ctx, "POST", fmt.Sprintf("/projects/%s/boards", projectID), body, &board); err != nil {
		return nil, err
	}
	return &board, nil
}

// UpdateBoard applies a merge patch (name and/or config) to a board.
func (c *Client) UpdateBoard(ctx context.Context, boardID string, patch map[string]any) (*Board, error) {
	var board Board
	if err := c.doItem(ctx, "PATCH", fmt.Sprintf("/boards/%s", boardID), patch, &board); err != nil {
		return nil, err
	}
	return &board, nil
}

// DeleteBoard permanently deletes a board and its tasks.
func (c *Client) DeleteBoard(ctx context.Context, boardID string) error {
	return c.doItem(ctx, "DELETE", fmt.Sprintf("/boards/%s", boardID), nil, nil)
}
