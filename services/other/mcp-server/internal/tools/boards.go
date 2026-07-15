package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/teamboard/services/other/mcp-server/internal/teamboard"
)

func registerBoardTools(server *mcp.Server, client *teamboard.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_boards",
		Description: "List boards. Pass project_id to scope to one project, or omit it to list boards across every project.",
	}, listBoards(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_board",
		Description: "Get a single board's detail, including its columns and board type.",
	}, getBoard(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_board",
		Description: "Create a board in a project. Works with every board type from list_board_types (kanban, scrum, calendar or custom-registered) — columns are seeded automatically from the type's default columns, and config is validated against the type's config_schema.",
	}, createBoard(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_board",
		Description: "Rename a board and/or replace its config. Only the fields you pass are changed.",
	}, updateBoard(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_board",
		Description: "Permanently delete a board and its tasks. Irreversible — confirm with the user before calling.",
	}, deleteBoard(client))
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
	BoardID string `json:"board_id" jsonschema:"The board ID"`
}

func getBoard(client *teamboard.Client) mcp.ToolHandlerFor[getBoardInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in getBoardInput) (*mcp.CallToolResult, any, error) {
		board, err := client.GetBoard(ctx, in.BoardID)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(board)
	}
}

// ── create_board ──────────────────────────────────────────────────────────────

type createBoardInput struct {
	ProjectID string         `json:"project_id" jsonschema:"The project to create the board in"`
	Name      string         `json:"name" jsonschema:"Board name"`
	Type      string         `json:"type" jsonschema:"Board type slug from list_board_types, e.g. kanban, scrum, calendar or a custom type"`
	Config    map[string]any `json:"config,omitempty" jsonschema:"Optional config overriding the type's default_config; validated against the type's config_schema"`
}

func createBoard(client *teamboard.Client) mcp.ToolHandlerFor[createBoardInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in createBoardInput) (*mcp.CallToolResult, any, error) {
		board, err := client.CreateBoard(ctx, in.ProjectID, in.Name, in.Type, in.Config)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(board)
	}
}

// ── update_board ──────────────────────────────────────────────────────────────

type updateBoardInput struct {
	BoardID string         `json:"board_id"`
	Name    *string        `json:"name,omitempty" jsonschema:"New board name; omit to keep"`
	Config  map[string]any `json:"config,omitempty" jsonschema:"New board config (replaces the current one); omit to keep"`
}

func updateBoard(client *teamboard.Client) mcp.ToolHandlerFor[updateBoardInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in updateBoardInput) (*mcp.CallToolResult, any, error) {
		patch := map[string]any{}
		if in.Name != nil {
			patch["name"] = *in.Name
		}
		if in.Config != nil {
			patch["config"] = in.Config
		}
		if len(patch) == 0 {
			return nil, nil, errNothingToUpdate
		}
		board, err := client.UpdateBoard(ctx, in.BoardID, patch)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(board)
	}
}

// ── delete_board ──────────────────────────────────────────────────────────────

type deleteBoardInput struct {
	BoardID string `json:"board_id" jsonschema:"The board to delete permanently"`
}

func deleteBoard(client *teamboard.Client) mcp.ToolHandlerFor[deleteBoardInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in deleteBoardInput) (*mcp.CallToolResult, any, error) {
		if err := client.DeleteBoard(ctx, in.BoardID); err != nil {
			return nil, nil, err
		}
		return jsonResult(map[string]string{"status": "deleted", "board_id": in.BoardID})
	}
}
