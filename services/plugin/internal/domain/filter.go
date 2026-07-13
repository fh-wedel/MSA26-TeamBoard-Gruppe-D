package domain

import (
	"strings"
	"unicode"
)

// MatchesFilter reports whether eventType matches any pattern in the filter list.
// Supported patterns: exact match, "prefix.*" wildcard, and global "*".
func MatchesFilter(eventType string, filter []string) bool {
	for _, pattern := range filter {
		if pattern == "*" || pattern == eventType {
			return true
		}
		if strings.HasSuffix(pattern, ".*") {
			prefix := pattern[:len(pattern)-1] // e.g. "task."
			if strings.HasPrefix(eventType, prefix) {
				return true
			}
		}
	}
	return false
}

var filterPatternValid = func(p string) bool {
	if p == "*" {
		return true
	}
	// Pattern: lowercase letters/digits/underscores, dots as separators, optional .* suffix.
	// e.g. "task.created", "task.*", "project.member.added"
	for i, r := range p {
		if r == '*' {
			// Only allowed as the very last character, preceded by a dot.
			return i == len(p)-1 && i > 0 && p[i-1] == '.'
		}
		if r != '.' && !unicode.IsLower(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return len(p) > 0
}

// ValidateEventFilter checks that each pattern in the filter is syntactically valid.
func ValidateEventFilter(filter []string) error {
	if len(filter) == 0 {
		return &Error{Code: ErrInvalidEventFilter.Code, Message: "event_filter must not be empty"}
	}
	for _, p := range filter {
		if !filterPatternValid(p) {
			return &Error{Code: ErrInvalidEventFilter.Code, Message: "invalid pattern: " + p}
		}
	}
	return nil
}
