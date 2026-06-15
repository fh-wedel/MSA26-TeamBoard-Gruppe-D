package domain

import (
	"bytes"
	"encoding/json"
	"regexp"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,49}$`)

// validateDefinition checks a board-type definition for structural validity and
// ensures the default config satisfies the config schema.
func validateDefinition(typ, displayName string, cols []ColumnDef, defaultConfig, schema map[string]any) error {
	if !slugRe.MatchString(typ) {
		return &Error{Code: ErrValidation.Code, Message: "type must be a lowercase slug (a-z, 0-9, _, -) starting with a letter, max 50 chars"}
	}
	if l := len(displayName); l < 1 || l > 100 {
		return &Error{Code: ErrValidation.Code, Message: "display_name must be 1..100 characters"}
	}

	seenPos := map[int]bool{}
	for _, c := range cols {
		if l := len(c.Name); l < 1 || l > 50 {
			return &Error{Code: ErrValidation.Code, Message: "column name must be 1..50 characters"}
		}
		if !ValidStatuses[c.Status] {
			return &Error{Code: ErrValidation.Code, Message: "invalid column status: " + c.Status}
		}
		if c.Position < 0 || seenPos[c.Position] {
			return &Error{Code: ErrValidation.Code, Message: "column positions must be unique and >= 0"}
		}
		seenPos[c.Position] = true
		if c.WIPLimit != nil && *c.WIPLimit < 0 {
			return &Error{Code: ErrValidation.Code, Message: "wip_limit must be >= 0"}
		}
	}

	compiled, err := CompileSchema(schema)
	if err != nil {
		return &Error{Code: ErrValidation.Code, Message: "invalid config_schema: " + err.Error()}
	}
	if compiled != nil && len(defaultConfig) > 0 {
		if err := compiled.Validate(toJSONValue(defaultConfig)); err != nil {
			return &Error{Code: ErrValidation.Code, Message: "default_config violates config_schema: " + err.Error()}
		}
	}
	return nil
}

// CompileSchema compiles a JSON-schema document. Returns (nil, nil) for an empty
// schema, which means "no validation".
func CompileSchema(schema map[string]any) (*jsonschema.Schema, error) {
	if len(schema) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", bytes.NewReader(raw)); err != nil {
		return nil, err
	}
	return c.Compile("schema.json")
}

// toJSONValue round-trips a map through encoding/json so the value is in the shape
// the jsonschema validator expects (map[string]any, []any, float64, ...).
func toJSONValue(m map[string]any) any {
	raw, _ := json.Marshal(m)
	var v any
	_ = json.Unmarshal(raw, &v)
	return v
}
