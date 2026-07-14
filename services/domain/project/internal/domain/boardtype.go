package domain

import "context"

// BoardTypeDef is a board-type definition resolved from the board registry service.
type BoardTypeDef struct {
	Type           string
	DisplayName    string
	Icon           string
	DefaultColumns []BoardTypeColumn
	DefaultConfig  map[string]any
	ConfigSchema   map[string]any
}

// BoardTypeColumn is a default column carried by a board-type definition,
// including its authoritative semantic status.
type BoardTypeColumn struct {
	Name     string
	Position int
	WIPLimit *int
	Status   string
}

// BoardTypeRegistry is the consumer-defined port for resolving board-type
// definitions at runtime. It is implemented by the boardtypeclient package,
// which talks to the board registry service over HTTP.
//
// GetType returns ErrInvalidBoardType when the type does not exist and
// ErrBoardTypeRegistryUnavailable when the registry cannot be reached.
type BoardTypeRegistry interface {
	GetType(ctx context.Context, typeID string) (*BoardTypeDef, error)
}
