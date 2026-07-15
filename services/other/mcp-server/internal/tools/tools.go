// Package tools registers the TeamBoard read/write tools against an MCP server.
// Tools are grouped by resource, mirroring internal/teamboard: projects.go,
// boards.go, boardtypes.go, tasks.go.
package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/teamboard/services/other/mcp-server/internal/teamboard"
)

// errNothingToUpdate is returned by update tools when the input names a target
// but no field to change — almost always a malformed model call.
var errNothingToUpdate = errors.New("nothing to update: pass at least one field to change")

// Register adds every TeamBoard tool to server, backed by client.
func Register(server *mcp.Server, client *teamboard.Client) {
	registerProjectTools(server, client)
	registerBoardTools(server, client)
	registerBoardTypeTools(server, client)
	registerTaskTools(server, client)
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

// parseDate accepts RFC 3339 ("2026-07-15T09:00:00Z") or a plain date
// ("2026-07-15", midnight UTC) — MCP clients commonly produce either. The
// TeamBoard API itself only takes RFC 3339 timestamps.
func parseDate(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid date %q: use YYYY-MM-DD or RFC 3339", s)
}

// optionalDate parses s when non-empty; an empty string means "not provided".
func optionalDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := parseDate(s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
