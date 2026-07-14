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

// GetBoard returns a single board with its columns.
func (c *Client) GetBoard(ctx context.Context, projectID, boardID string) (*Board, error) {
	var board Board
	if err := c.doItem(ctx, "GET", fmt.Sprintf("/projects/%s/boards/%s", projectID, boardID), nil, &board); err != nil {
		return nil, err
	}
	return &board, nil
}
