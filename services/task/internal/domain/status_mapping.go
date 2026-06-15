package domain

import "strings"

// DeriveStatus maps a column name to a semantic task status.
// Uses best-effort keyword matching; explicit status from caller takes precedence.
func DeriveStatus(columnName string) Status {
	name := strings.ToLower(columnName)
	switch {
	case strings.Contains(name, "done") || strings.Contains(name, "complete"):
		return StatusDone
	case strings.Contains(name, "progress") || strings.Contains(name, "doing"):
		return StatusInProgress
	case strings.Contains(name, "block"):
		return StatusBlocked
	case strings.Contains(name, "archive"):
		return StatusArchived
	default:
		return StatusOpen
	}
}
