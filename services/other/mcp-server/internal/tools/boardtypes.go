package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/teamboard/services/other/mcp-server/internal/teamboard"
)

func registerBoardTypeTools(server *mcp.Server, client *teamboard.Client) {
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
