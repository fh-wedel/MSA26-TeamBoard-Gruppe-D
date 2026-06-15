package calendar

import (
	"fmt"

	"github.com/teamboard/services/project/internal/boardplugins"
)

func init() {
	boardplugins.Register(boardplugins.Plugin{
		Type:           "calendar",
		DisplayName:    "Calendar",
		Icon:           "📅",
		DefaultColumns: func() []boardplugins.ColumnInput { return nil },
		DefaultConfig:  func() map[string]any { return map[string]any{"week_start": "monday"} },
		ValidateConfig: func(c map[string]any) error {
			if v, ok := c["week_start"]; ok {
				s, ok := v.(string)
				if !ok || (s != "monday" && s != "sunday") {
					return fmt.Errorf("week_start must be 'monday' or 'sunday'")
				}
			}
			return nil
		},
	})
}
