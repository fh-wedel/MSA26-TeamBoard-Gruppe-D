package teamboard

import (
	"context"
	"fmt"
)

// ListBoardTypes returns the full board-type catalog (built-in + custom).
func (c *Client) ListBoardTypes(ctx context.Context) ([]BoardTypeDef, error) {
	var defs []BoardTypeDef
	if _, err := c.doList(ctx, "GET", "/board-types", &defs); err != nil {
		return nil, err
	}
	return defs, nil
}

// RegisterBoardTypeInput mirrors services/boardregistry/internal/api/dto.go's registerRequest.
type RegisterBoardTypeInput struct {
	Type           string         `json:"type"`
	DisplayName    string         `json:"display_name"`
	Icon           string         `json:"icon"`
	DefaultColumns []ColumnDef    `json:"default_columns"`
	DefaultConfig  map[string]any `json:"default_config"`
	ConfigSchema   map[string]any `json:"config_schema"`
	Presentation   map[string]any `json:"presentation,omitempty"`
}

// RegisterBoardType creates a new custom board type in the registry.
func (c *Client) RegisterBoardType(ctx context.Context, in RegisterBoardTypeInput) (*BoardTypeDef, error) {
	var def BoardTypeDef
	if err := c.doItem(ctx, "POST", "/board-types", in, &def); err != nil {
		return nil, err
	}
	return &def, nil
}

// DeleteBoardType removes a custom board type. Built-in types cannot be deleted.
func (c *Client) DeleteBoardType(ctx context.Context, boardType string) error {
	return c.doItem(ctx, "DELETE", fmt.Sprintf("/board-types/%s", boardType), nil, nil)
}
