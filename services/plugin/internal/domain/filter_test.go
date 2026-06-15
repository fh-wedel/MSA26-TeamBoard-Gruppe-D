package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/teamboard/services/plugin/internal/domain"
)

func TestMatchesFilter(t *testing.T) {
	cases := []struct {
		name      string
		filter    []string
		eventType string
		want      bool
	}{
		{"exact match", []string{"task.created"}, "task.created", true},
		{"prefix wildcard", []string{"task.*"}, "task.updated", true},
		{"prefix wildcard no match", []string{"task.*"}, "project.created", false},
		{"global wildcard", []string{"*"}, "anything.goes", true},
		{"no match", []string{"task.created"}, "project.created", false},
		{"multiple, one matches", []string{"task.created", "task.updated"}, "task.updated", true},
		{"multiple, none match", []string{"task.created", "project.created"}, "board.created", false},
		{"empty filter", []string{}, "task.created", false},
		{"board wildcard", []string{"board.*"}, "board.column.added", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.MatchesFilter(tc.eventType, tc.filter)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestValidateEventFilter(t *testing.T) {
	t.Run("valid patterns", func(t *testing.T) {
		err := domain.ValidateEventFilter([]string{"task.created", "task.*", "*", "project.member.added"})
		assert.NoError(t, err)
	})
	t.Run("empty filter rejected", func(t *testing.T) {
		err := domain.ValidateEventFilter([]string{})
		assert.Error(t, err)
	})
	t.Run("invalid pattern with uppercase", func(t *testing.T) {
		err := domain.ValidateEventFilter([]string{"Task.Created"})
		assert.Error(t, err)
	})
	t.Run("wildcard in middle rejected", func(t *testing.T) {
		err := domain.ValidateEventFilter([]string{"task.*.created"})
		assert.Error(t, err)
	})
}
