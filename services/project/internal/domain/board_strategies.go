package domain

// BoardTypeStrategy and StrategyFor are kept for backward compatibility with tests.
// New board types should be registered via boardplugins.Register() instead.

// BoardTypeStrategy provides type-specific defaults and validation.
type BoardTypeStrategy interface {
	DefaultColumns() []BoardColumnInput
	DefaultConfig() map[string]any
	ValidateConfig(config map[string]any) error
}
