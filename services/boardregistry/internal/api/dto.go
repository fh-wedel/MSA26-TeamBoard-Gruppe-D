package api

import (
	"time"

	"github.com/google/uuid"

	"github.com/teamboard/services/boardregistry/internal/domain"
)

// ── Requests ──────────────────────────────────────────────────────────────────

type columnDefDTO struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
	WIPLimit *int   `json:"wip_limit,omitempty"`
	Status   string `json:"status"`
}

type registerRequest struct {
	Type           string         `json:"type"`
	DisplayName    string         `json:"display_name"`
	Icon           string         `json:"icon"`
	DefaultColumns []columnDefDTO `json:"default_columns"`
	DefaultConfig  map[string]any `json:"default_config"`
	ConfigSchema   map[string]any `json:"config_schema"`
}

type updateRequest struct {
	DisplayName    *string         `json:"display_name"`
	Icon           *string         `json:"icon"`
	DefaultColumns *[]columnDefDTO `json:"default_columns"`
	DefaultConfig  *map[string]any `json:"default_config"`
	ConfigSchema   *map[string]any `json:"config_schema"`
}

// ── Responses ─────────────────────────────────────────────────────────────────

type boardTypeResponse struct {
	Type           string         `json:"type"`
	DisplayName    string         `json:"display_name"`
	Icon           string         `json:"icon"`
	DefaultColumns []columnDefDTO `json:"default_columns"`
	DefaultConfig  map[string]any `json:"default_config"`
	ConfigSchema   map[string]any `json:"config_schema"`
	BuiltIn        bool           `json:"built_in"`
	CreatedBy      *uuid.UUID     `json:"created_by,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// ── Mapping ───────────────────────────────────────────────────────────────────

func toColumnDefs(in []columnDefDTO) []domain.ColumnDef {
	out := make([]domain.ColumnDef, len(in))
	for i, c := range in {
		out[i] = domain.ColumnDef{Name: c.Name, Position: c.Position, WIPLimit: c.WIPLimit, Status: c.Status}
	}
	return out
}

func fromColumnDefs(in []domain.ColumnDef) []columnDefDTO {
	out := make([]columnDefDTO, len(in))
	for i, c := range in {
		out[i] = columnDefDTO{Name: c.Name, Position: c.Position, WIPLimit: c.WIPLimit, Status: c.Status}
	}
	return out
}

func toResponse(d *domain.BoardTypeDef) boardTypeResponse {
	return boardTypeResponse{
		Type:           d.Type,
		DisplayName:    d.DisplayName,
		Icon:           d.Icon,
		DefaultColumns: fromColumnDefs(d.DefaultColumns),
		DefaultConfig:  d.DefaultConfig,
		ConfigSchema:   d.ConfigSchema,
		BuiltIn:        d.BuiltIn,
		CreatedBy:      d.CreatedBy,
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
	}
}
