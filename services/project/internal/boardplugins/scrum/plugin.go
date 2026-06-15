package scrum

import (
	"fmt"

	"github.com/teamboard/services/project/internal/boardplugins"
)

func init() {
	boardplugins.Register(boardplugins.Plugin{
		Type:        "scrum",
		DisplayName: "Scrum Board",
		Icon:        "🏃",
		DefaultColumns: func() []boardplugins.ColumnInput {
			return []boardplugins.ColumnInput{
				{Name: "Backlog", Position: 0},
				{Name: "Sprint", Position: 1},
				{Name: "In Progress", Position: 2},
				{Name: "Review", Position: 3},
				{Name: "Done", Position: 4},
			}
		},
		DefaultConfig: func() map[string]any {
			return map[string]any{"sprint_length_days": float64(14)}
		},
		ValidateConfig: func(c map[string]any) error {
			if v, ok := c["sprint_length_days"]; ok {
				n, ok := v.(float64)
				if !ok || n < 1 || n > 90 {
					return fmt.Errorf("sprint_length_days must be between 1 and 90")
				}
			}
			return nil
		},
	})
}
