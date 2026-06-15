package domain

import (
	"time"

	"github.com/google/uuid"
)

// ColumnDef is a default column carried by a board-type definition. Status is the
// authoritative semantic task status for tasks in this column (open, in_progress,
// blocked, done, archived) — this removes the need for the task service to guess
// the status from the column name.
type ColumnDef struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
	WIPLimit *int   `json:"wip_limit,omitempty"`
	Status   string `json:"status"`
}

// BoardTypeDef is a registered board-type definition. The slug Type is the public
// identifier used by the project service when creating boards.
type BoardTypeDef struct {
	ID             uuid.UUID
	Type           string
	DisplayName    string
	Icon           string
	DefaultColumns []ColumnDef
	DefaultConfig  map[string]any
	ConfigSchema   map[string]any // JSON Schema used to validate board config
	BuiltIn        bool
	CreatedBy      *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type RegisterInput struct {
	Type           string
	DisplayName    string
	Icon           string
	DefaultColumns []ColumnDef
	DefaultConfig  map[string]any
	ConfigSchema   map[string]any
	CreatedBy      *uuid.UUID
}

type UpdatePatch struct {
	DisplayName    *string
	Icon           *string
	DefaultColumns *[]ColumnDef
	DefaultConfig  *map[string]any
	ConfigSchema   *map[string]any
}

// ValidStatuses must match the task service's status set (see services/task status_mapping.go).
var ValidStatuses = map[string]bool{
	"open":        true,
	"in_progress": true,
	"blocked":     true,
	"done":        true,
	"archived":    true,
}

type OutboxEvent struct {
	ID          uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	Payload     []byte
	OccurredAt  time.Time
	PublishedAt *time.Time
}
