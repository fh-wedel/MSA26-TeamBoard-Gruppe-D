// Package tools registers the TeamBoard read/write tools against an MCP server.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/teamboard/services/other/mcp-server/internal/teamboard"
)

// Register adds every TeamBoard tool to server, backed by client.
func Register(server *mcp.Server, client *teamboard.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_projects",
		Description: "List all TeamBoard projects the authenticated user is a member of.",
	}, listProjects(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_boards",
		Description: "List boards. Pass project_id to scope to one project, or omit it to list boards across every project.",
	}, listBoards(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_board",
		Description: "Get a single board's detail, including its columns and board type.",
	}, getBoard(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tasks",
		Description: "List tasks on a board, optionally filtered by status (open, in_progress, blocked, done, archived) and/or column.",
	}, listTasks(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_task",
		Description: "Get full detail for a single task, including due date, assignee and labels.",
	}, getTask(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_board_types",
		Description: "List the board-type catalog (built-in types like kanban/scrum/calendar plus any custom-registered types).",
	}, listBoardTypes(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "register_board_type",
		Description: "Register a new custom board type in the Board Registry, e.g. a Gantt/timeline board. Mirrors what docs/demo/presentation-board-registry.ipynb does interactively.",
	}, registerBoardType(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_board_type",
		Description: "Delete a custom board type by its type slug. Built-in board types cannot be deleted.",
	}, deleteBoardType(client))
}

// jsonResult serializes v as indented JSON text content — the simplest way
// for a model to read structured TeamBoard data back from a tool call.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("marshal result: %w", err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}

// ── list_projects ────────────────────────────────────────────────────────────

type listProjectsInput struct{}

func listProjects(client *teamboard.Client) mcp.ToolHandlerFor[listProjectsInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ listProjectsInput) (*mcp.CallToolResult, any, error) {
		projects, err := client.ListProjects(ctx)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(projects)
	}
}

// ── list_boards ───────────────────────────────────────────────────────────────

type listBoardsInput struct {
	ProjectID string `json:"project_id,omitempty" jsonschema:"Only list boards in this project; omit to list boards across all projects the user can see"`
}

func listBoards(client *teamboard.Client) mcp.ToolHandlerFor[listBoardsInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in listBoardsInput) (*mcp.CallToolResult, any, error) {
		if in.ProjectID != "" {
			boards, err := client.ListBoards(ctx, in.ProjectID)
			if err != nil {
				return nil, nil, err
			}
			return jsonResult(boards)
		}

		projects, err := client.ListProjects(ctx)
		if err != nil {
			return nil, nil, err
		}
		var all []teamboard.Board
		for _, p := range projects {
			boards, err := client.ListBoards(ctx, p.ID)
			if err != nil {
				return nil, nil, err
			}
			all = append(all, boards...)
		}
		return jsonResult(all)
	}
}

// ── get_board ─────────────────────────────────────────────────────────────────

type getBoardInput struct {
	ProjectID string `json:"project_id" jsonschema:"The board's parent project ID"`
	BoardID   string `json:"board_id" jsonschema:"The board ID"`
}

func getBoard(client *teamboard.Client) mcp.ToolHandlerFor[getBoardInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in getBoardInput) (*mcp.CallToolResult, any, error) {
		board, err := client.GetBoard(ctx, in.ProjectID, in.BoardID)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(board)
	}
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

// ── list_board_types ────────────────────────────────────────────────────────

type listBoardTypesInput struct{}

func listBoardTypes(client *teamboard.Client) mcp.ToolHandlerFor[listBoardTypesInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ listBoardTypesInput) (*mcp.CallToolResult, any, error) {
		defs, err := client.ListBoardTypes(ctx)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(defs)
	}
}

// ── register_board_type ──────────────────────────────────────────────────────

type columnDefInput struct {
	Name     string `json:"name" jsonschema:"Column display name, e.g. 'In Progress'"`
	Position int    `json:"position" jsonschema:"0-based column order"`
	WIPLimit *int   `json:"wip_limit,omitempty" jsonschema:"Optional work-in-progress limit"`
	Status   string `json:"status" jsonschema:"Semantic status this column maps to: open, in_progress, blocked, done, archived"`
}

type registerBoardTypeInput struct {
	Type           string           `json:"type" jsonschema:"URL-safe slug for the new board type, e.g. 'gantt'"`
	DisplayName    string           `json:"display_name" jsonschema:"Human-readable name, e.g. 'Gantt (Timeline)'"`
	Icon           string           `json:"icon,omitempty" jsonschema:"An emoji to represent the board type"`
	DefaultColumns []columnDefInput `json:"default_columns" jsonschema:"Columns a new board of this type starts with"`
	DefaultConfig  map[string]any   `json:"default_config,omitempty"`
	ConfigSchema   map[string]any   `json:"config_schema,omitempty" jsonschema:"JSON-Schema fragment validating this board type's config"`
	Presentation   map[string]any   `json:"presentation,omitempty" jsonschema:"Declarative view spec: {view, view_config, card} — view is one of board, calendar, timeline"`
}

func registerBoardType(client *teamboard.Client) mcp.ToolHandlerFor[registerBoardTypeInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in registerBoardTypeInput) (*mcp.CallToolResult, any, error) {
		cols := make([]teamboard.ColumnDef, len(in.DefaultColumns))
		for i, c := range in.DefaultColumns {
			cols[i] = teamboard.ColumnDef{Name: c.Name, Position: c.Position, WIPLimit: c.WIPLimit, Status: c.Status}
		}
		def, err := client.RegisterBoardType(ctx, teamboard.RegisterBoardTypeInput{
			Type:           in.Type,
			DisplayName:    in.DisplayName,
			Icon:           in.Icon,
			DefaultColumns: cols,
			DefaultConfig:  in.DefaultConfig,
			ConfigSchema:   in.ConfigSchema,
			Presentation:   in.Presentation,
		})
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(def)
	}
}

// ── delete_board_type ─────────────────────────────────────────────────────────

type deleteBoardTypeInput struct {
	Type string `json:"type" jsonschema:"The board type slug to delete, e.g. 'gantt'"`
}

func deleteBoardType(client *teamboard.Client) mcp.ToolHandlerFor[deleteBoardTypeInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in deleteBoardTypeInput) (*mcp.CallToolResult, any, error) {
		if err := client.DeleteBoardType(ctx, in.Type); err != nil {
			return nil, nil, err
		}
		return jsonResult(map[string]string{"status": "deleted", "type": in.Type})
	}
}
