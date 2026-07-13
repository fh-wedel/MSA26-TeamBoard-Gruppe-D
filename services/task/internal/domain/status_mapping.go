package domain

import "strings"

// statusForColumn returns the column's explicit semantic status when set. It
// falls back to keyword derivation from the column name for legacy columns that
// predate the explicit board-type status mapping.
func statusForColumn(col *KnownColumn) Status {
	if col != nil && col.Status != "" {
		if s := Status(col.Status); ValidStatus(s) {
			return s
		}
	}
	if col == nil {
		return StatusOpen
	}
	return DeriveStatus(col.Name)
}

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
