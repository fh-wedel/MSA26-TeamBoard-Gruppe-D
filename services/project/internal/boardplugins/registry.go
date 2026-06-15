// Package boardplugins implements a self-registering plugin registry for board types.
// Each board type is a separate package under boardplugins/<name>/ that calls Register
// in its init() function. main.go activates plugins with blank imports.
package boardplugins

import "sync"

// ColumnInput describes a column to be created as part of board initialisation.
type ColumnInput struct {
	Name     string
	Position int
	WIPLimit *int
}

// Plugin is the contract every board type must satisfy.
type Plugin struct {
	Type           string
	DisplayName    string
	Icon           string
	DefaultColumns func() []ColumnInput
	DefaultConfig  func() map[string]any
	ValidateConfig func(map[string]any) error
}

var (
	mu       sync.RWMutex
	registry = map[string]Plugin{}
)

// Register adds p to the global registry. It is called from plugin init() functions.
func Register(p Plugin) {
	mu.Lock()
	defer mu.Unlock()
	registry[p.Type] = p
}

// Get returns the plugin for the given board type, and false if not registered.
func Get(boardType string) (Plugin, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := registry[boardType]
	return p, ok
}

// List returns all registered plugins in an unspecified order.
func List() []Plugin {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Plugin, 0, len(registry))
	for _, p := range registry {
		out = append(out, p)
	}
	return out
}
