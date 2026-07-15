package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/teamboard/services/other/mcp-server/internal/teamboard"
)

func registerTaskTools(server *mcp.Server, client *teamboard.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tasks",
		Description: "List tasks on a board, optionally filtered by status (open, in_progress, blocked, done, archived) and/or column.",
	}, listTasks(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_task",
		Description: "Get full detail for a single task, including due date, assignee and labels.",
	}, getTask(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_task",
		Description: "Create a task on a board (kanban, calendar or any other type). Only title is required; priority defaults to medium. On boards with columns the task lands in the first column unless column_id says otherwise; calendar boards need no column but usually want start_date/due_date.",
	}, createTask(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_task",
		Description: "Edit a task's title, description, priority, status, start/due date or labels. Only the fields you pass are changed. Note: on a kanban board, changing status here does NOT move the card to another column — use move_task for that.",
	}, updateTask(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "move_task",
		Description: "Move a task into another column on its board (the kanban drag&drop). The task's status is updated automatically from the destination column's semantic status — this is the right tool to 'put a task in another status' on a kanban board. Use get_board to see the board's columns.",
	}, moveTask(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "assign_task",
		Description: "Assign a task to a project member, or unassign it. Use list_project_members to find user IDs.",
	}, assignTask(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_task",
		Description: "Permanently delete a task with its comments and attachments. Irreversible — confirm with the user before calling.",
	}, deleteTask(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_comments",
		Description: "List the comments on a task, oldest first.",
	}, listComments(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_comment",
		Description: "Write a comment on a task.",
	}, createComment(client))
}

// ── list_tasks ────────────────────────────────────────────────────────────────

type listTasksInput struct {
	BoardID  string `json:"board_id" jsonschema:"The board to list tasks on"`
	Status   string `json:"status,omitempty" jsonschema:"Filter by status: open, in_progress, blocked, done, archived"`
	ColumnID string `json:"column_id,omitempty" jsonschema:"Filter to a single column"`
}

func listTasks(client *teamboard.Client) mcp.ToolHandlerFor[listTasksInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in listTasksInput) (*mcp.CallToolResult, any, error) {
		tasks, err := client.ListTasks(ctx, in.BoardID, in.Status, in.ColumnID)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(tasks)
	}
}

// ── get_task ──────────────────────────────────────────────────────────────────

type getTaskInput struct {
	TaskID string `json:"task_id"`
}

func getTask(client *teamboard.Client) mcp.ToolHandlerFor[getTaskInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in getTaskInput) (*mcp.CallToolResult, any, error) {
		task, err := client.GetTask(ctx, in.TaskID)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(task)
	}
}

// ── create_task ───────────────────────────────────────────────────────────────

type createTaskInput struct {
	BoardID     string   `json:"board_id" jsonschema:"The board to create the task on"`
	Title       string   `json:"title" jsonschema:"Task title (required, max 500 chars)"`
	Description string   `json:"description,omitempty" jsonschema:"Optional longer description"`
	Priority    string   `json:"priority,omitempty" jsonschema:"low, medium, high or critical; defaults to medium"`
	ColumnID    string   `json:"column_id,omitempty" jsonschema:"Column to place the task in; defaults to the board's first column. Boards without columns (calendar) don't need one"`
	AssigneeID  string   `json:"assignee_id,omitempty" jsonschema:"Optional user ID to assign (see list_project_members)"`
	StartDate   string   `json:"start_date,omitempty" jsonschema:"Optional start date, YYYY-MM-DD or RFC 3339"`
	DueDate     string   `json:"due_date,omitempty" jsonschema:"Optional due date, YYYY-MM-DD or RFC 3339"`
	Labels      []string `json:"labels,omitempty" jsonschema:"Optional labels"`
}

func createTask(client *teamboard.Client) mcp.ToolHandlerFor[createTaskInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in createTaskInput) (*mcp.CallToolResult, any, error) {
		startDate, err := optionalDate(in.StartDate)
		if err != nil {
			return nil, nil, fmt.Errorf("start_date: %w", err)
		}
		dueDate, err := optionalDate(in.DueDate)
		if err != nil {
			return nil, nil, fmt.Errorf("due_date: %w", err)
		}

		input := teamboard.CreateTaskInput{
			Title:       in.Title,
			Description: in.Description,
			Priority:    in.Priority,
			StartDate:   startDate,
			DueDate:     dueDate,
			Labels:      in.Labels,
		}
		if in.AssigneeID != "" {
			input.AssigneeID = &in.AssigneeID
		}

		// Default to the board's first column so the task is visible on column
		// boards (the web UI always creates into a column). Boards without
		// columns — e.g. calendar — simply get a column-less task.
		columnID := in.ColumnID
		if columnID == "" {
			board, err := client.GetBoard(ctx, in.BoardID)
			if err != nil {
				return nil, nil, err
			}
			if len(board.Columns) > 0 {
				first := board.Columns[0]
				for _, col := range board.Columns[1:] {
					if col.Position < first.Position {
						first = col
					}
				}
				columnID = first.ID
			}
		}
		if columnID != "" {
			input.ColumnID = &columnID
		}

		task, err := client.CreateTask(ctx, in.BoardID, input)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(task)
	}
}

// ── update_task ───────────────────────────────────────────────────────────────

type updateTaskInput struct {
	TaskID         string   `json:"task_id"`
	Title          *string  `json:"title,omitempty" jsonschema:"New title; omit to keep"`
	Description    *string  `json:"description,omitempty" jsonschema:"New description (pass an empty string to clear); omit to keep"`
	Priority       string   `json:"priority,omitempty" jsonschema:"New priority: low, medium, high, critical"`
	Status         string   `json:"status,omitempty" jsonschema:"New status: open, in_progress, blocked, done, archived. On column boards prefer move_task, which also moves the card"`
	StartDate      string   `json:"start_date,omitempty" jsonschema:"New start date, YYYY-MM-DD or RFC 3339"`
	DueDate        string   `json:"due_date,omitempty" jsonschema:"New due date, YYYY-MM-DD or RFC 3339"`
	ClearStartDate bool     `json:"clear_start_date,omitempty" jsonschema:"Set true to remove the start date"`
	ClearDueDate   bool     `json:"clear_due_date,omitempty" jsonschema:"Set true to remove the due date"`
	Labels         []string `json:"labels,omitempty" jsonschema:"Replacement label list (pass [] to clear); omit to keep"`
}

func updateTask(client *teamboard.Client) mcp.ToolHandlerFor[updateTaskInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in updateTaskInput) (*mcp.CallToolResult, any, error) {
		patch := map[string]any{}
		if in.Title != nil {
			patch["title"] = *in.Title
		}
		if in.Description != nil {
			patch["description"] = *in.Description
		}
		if in.Priority != "" {
			patch["priority"] = in.Priority
		}
		if in.Status != "" {
			patch["status"] = in.Status
		}
		if in.ClearStartDate {
			patch["start_date"] = nil
		} else if in.StartDate != "" {
			t, err := parseDate(in.StartDate)
			if err != nil {
				return nil, nil, fmt.Errorf("start_date: %w", err)
			}
			patch["start_date"] = t
		}
		if in.ClearDueDate {
			patch["due_date"] = nil
		} else if in.DueDate != "" {
			t, err := parseDate(in.DueDate)
			if err != nil {
				return nil, nil, fmt.Errorf("due_date: %w", err)
			}
			patch["due_date"] = t
		}
		if in.Labels != nil {
			patch["labels"] = in.Labels
		}
		if len(patch) == 0 {
			return nil, nil, errNothingToUpdate
		}

		task, err := client.UpdateTask(ctx, in.TaskID, patch)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(task)
	}
}

// ── move_task ─────────────────────────────────────────────────────────────────

type moveTaskInput struct {
	TaskID   string `json:"task_id"`
	ColumnID string `json:"column_id" jsonschema:"Destination column on the task's board; the task's status follows the column's semantic status"`
	BeforeID string `json:"before_id,omitempty" jsonschema:"Optional: existing task that should sit immediately before the moved task"`
	AfterID  string `json:"after_id,omitempty" jsonschema:"Optional: existing task that should sit immediately after the moved task"`
}

func moveTask(client *teamboard.Client) mcp.ToolHandlerFor[moveTaskInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in moveTaskInput) (*mcp.CallToolResult, any, error) {
		var beforeID, afterID *string
		if in.BeforeID != "" {
			beforeID = &in.BeforeID
		}
		if in.AfterID != "" {
			afterID = &in.AfterID
		}
		task, err := client.MoveTask(ctx, in.TaskID, in.ColumnID, beforeID, afterID)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(task)
	}
}

// ── assign_task ───────────────────────────────────────────────────────────────

type assignTaskInput struct {
	TaskID     string `json:"task_id"`
	AssigneeID string `json:"assignee_id,omitempty" jsonschema:"User ID to assign (see list_project_members); omit or pass empty to unassign"`
}

func assignTask(client *teamboard.Client) mcp.ToolHandlerFor[assignTaskInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in assignTaskInput) (*mcp.CallToolResult, any, error) {
		var assigneeID *string
		if in.AssigneeID != "" {
			assigneeID = &in.AssigneeID
		}
		task, err := client.AssignTask(ctx, in.TaskID, assigneeID)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(task)
	}
}

// ── delete_task ───────────────────────────────────────────────────────────────

type deleteTaskInput struct {
	TaskID string `json:"task_id" jsonschema:"The task to delete permanently"`
}

func deleteTask(client *teamboard.Client) mcp.ToolHandlerFor[deleteTaskInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in deleteTaskInput) (*mcp.CallToolResult, any, error) {
		if err := client.DeleteTask(ctx, in.TaskID); err != nil {
			return nil, nil, err
		}
		return jsonResult(map[string]string{"status": "deleted", "task_id": in.TaskID})
	}
}

// ── list_comments ─────────────────────────────────────────────────────────────

type listCommentsInput struct {
	TaskID string `json:"task_id"`
}

func listComments(client *teamboard.Client) mcp.ToolHandlerFor[listCommentsInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in listCommentsInput) (*mcp.CallToolResult, any, error) {
		comments, err := client.ListComments(ctx, in.TaskID)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(comments)
	}
}

// ── create_comment ────────────────────────────────────────────────────────────

type createCommentInput struct {
	TaskID string `json:"task_id"`
	Body   string `json:"body" jsonschema:"The comment text"`
}

func createComment(client *teamboard.Client) mcp.ToolHandlerFor[createCommentInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in createCommentInput) (*mcp.CallToolResult, any, error) {
		comment, err := client.CreateComment(ctx, in.TaskID, in.Body)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(comment)
	}
}
