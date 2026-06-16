package domain

import (
	"bytes"
	"encoding/json"
	"regexp"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,49}$`)

// ValidViews are the built-in frontend renderers that presentation.view may select.
// This is a host-defined capability set: a definition may only choose a renderer the
// frontend actually ships (unknown values fall back to "board" with a notice).
var ValidViews = map[string]bool{
	"board":    true,
	"calendar": true,
	"timeline": true,
}

// allowedViewConfigKeys lists the recognised view_config keys per view, so a
// definition cannot smuggle in typos or fields no renderer understands.
var allowedViewConfigKeys = map[string]map[string]bool{
	"board":    {"group_by": true, "show_wip": true, "swimlane_by": true},
	"calendar": {"date_field": true, "week_start": true, "default_range": true},
	"timeline": {"start_field": true, "end_field": true, "group_by": true, "color_by": true},
}

var validColorBy = map[string]bool{"priority": true, "status": true, "label": true}

// validatePresentation checks the (optional) presentation spec against the host's
// meta-schema. An empty spec is valid and means the default board renderer.
func validatePresentation(p map[string]any) error {
	if len(p) == 0 {
		return nil
	}
	verr := func(msg string) error { return &Error{Code: ErrValidation.Code, Message: msg} }

	// Reject unknown top-level keys.
	for k := range p {
		switch k {
		case "view", "view_config", "card":
		default:
			return verr("presentation: unknown key '" + k + "'")
		}
	}

	view := "board"
	if v, ok := p["view"]; ok {
		s, isStr := v.(string)
		if !isStr || !ValidViews[s] {
			return verr("presentation.view must be one of board, calendar, timeline")
		}
		view = s
	}

	if vc, ok := p["view_config"]; ok {
		m, isObj := vc.(map[string]any)
		if !isObj {
			return verr("presentation.view_config must be an object")
		}
		allowed := allowedViewConfigKeys[view]
		for k := range m {
			if !allowed[k] {
				return verr("presentation.view_config: key '" + k + "' is not valid for view '" + view + "'")
			}
		}
		if ws, ok := m["week_start"].(string); ok && ws != "monday" && ws != "sunday" {
			return verr("presentation.view_config.week_start must be monday or sunday")
		}
		if cb, ok := m["color_by"].(string); ok && !validColorBy[cb] {
			return verr("presentation.view_config.color_by must be priority, status or label")
		}
	}

	if c, ok := p["card"]; ok {
		m, isObj := c.(map[string]any)
		if !isObj {
			return verr("presentation.card must be an object")
		}
		for k := range m {
			if k != "fields" && k != "color_by" {
				return verr("presentation.card: unknown key '" + k + "'")
			}
		}
		if f, ok := m["fields"]; ok {
			arr, isArr := f.([]any)
			if !isArr {
				return verr("presentation.card.fields must be an array of strings")
			}
			for _, e := range arr {
				if _, isStr := e.(string); !isStr {
					return verr("presentation.card.fields must be an array of strings")
				}
			}
		}
		if cb, ok := m["color_by"].(string); ok && !validColorBy[cb] {
			return verr("presentation.card.color_by must be priority, status or label")
		}
	}
	return nil
}

// validateDefinition checks a board-type definition for structural validity and
// ensures the default config satisfies the config schema.
func validateDefinition(typ, displayName string, cols []ColumnDef, defaultConfig, schema, presentation map[string]any) error {
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
	return validatePresentation(presentation)
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
