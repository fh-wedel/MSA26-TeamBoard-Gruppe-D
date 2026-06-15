package httputil

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// EncodeCursor base64url-encodes any JSON-serialisable cursor payload.
func EncodeCursor(payload any) (string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// DecodeCursor decodes a base64url cursor into target (must be a pointer).
// Empty cursor is a no-op.
func DecodeCursor(cursor string, target any) error {
	if cursor == "" {
		return nil
	}
	b, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return fmt.Errorf("invalid cursor: %w", err)
	}
	return json.Unmarshal(b, target)
}
