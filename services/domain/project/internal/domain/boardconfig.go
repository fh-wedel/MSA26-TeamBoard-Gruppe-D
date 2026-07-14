package domain

import (
	"bytes"
	"encoding/json"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// ValidateBoardConfig validates a board config against the board type's JSON
// schema. An empty schema means "no validation". Returns ErrValidation on a
// schema violation.
func ValidateBoardConfig(schema, config map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return &Error{Code: ErrValidation.Code, Message: "invalid config schema", Cause: err}
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", bytes.NewReader(raw)); err != nil {
		return &Error{Code: ErrValidation.Code, Message: "invalid config schema", Cause: err}
	}
	compiled, err := c.Compile("schema.json")
	if err != nil {
		return &Error{Code: ErrValidation.Code, Message: "invalid config schema", Cause: err}
	}

	// Round-trip the config so values are in the shape the validator expects.
	cfgRaw, _ := json.Marshal(config)
	var value any
	_ = json.Unmarshal(cfgRaw, &value)

	if err := compiled.Validate(value); err != nil {
		return &Error{Code: ErrValidation.Code, Message: "config violates board type schema: " + err.Error()}
	}
	return nil
}
