package kanban

import "github.com/teamboard/services/project/internal/boardplugins"

func init() {
	wip := 3
	boardplugins.Register(boardplugins.Plugin{
		Type:        "kanban",
		DisplayName: "Kanban Board",
		Icon:        "📋",
		DefaultColumns: func() []boardplugins.ColumnInput {
			return []boardplugins.ColumnInput{
				{Name: "To Do", Position: 0},
				{Name: "In Progress", Position: 1, WIPLimit: &wip},
				{Name: "Done", Position: 2},
			}
		},
		DefaultConfig:  func() map[string]any { return map[string]any{} },
		ValidateConfig: func(_ map[string]any) error { return nil },
	})
}
