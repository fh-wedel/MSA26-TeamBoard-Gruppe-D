package teamboard

import "time"

// Project mirrors services/project/internal/api/dto.go's projectResponse.
type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	OwnerID     string    `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Column mirrors services/project/internal/api/dto.go's columnResponse.
type Column struct {
	ID       string `json:"id"`
	BoardID  string `json:"board_id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	WIPLimit *int   `json:"wip_limit,omitempty"`
	Status   string `json:"status,omitempty"`
}

// Board mirrors services/project/internal/api/dto.go's boardResponse.
type Board struct {
	ID        string         `json:"id"`
	ProjectID string         `json:"project_id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Position  int            `json:"position"`
	Config    map[string]any `json:"config"`
	CreatedBy string         `json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Columns   []Column       `json:"columns,omitempty"`
}

// Task mirrors services/task/internal/api/dto.go's taskResponse.
type Task struct {
	ID              string     `json:"id"`
	BoardID         string     `json:"board_id"`
	ProjectID       string     `json:"project_id"`
	ColumnID        *string    `json:"column_id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	Status          string     `json:"status"`
	Priority        string     `json:"priority"`
	AssigneeID      *string    `json:"assignee_id"`
	DueDate         *time.Time `json:"due_date"`
	StartDate       *time.Time `json:"start_date"`
	Labels          []string   `json:"labels"`
	Position        string     `json:"position"`
	CreatedBy       string     `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CommentCount    int        `json:"comment_count"`
	AttachmentCount int        `json:"attachment_count"`
}

// Comment mirrors services/task/internal/api/dto.go's commentResponse.
type Comment struct {
	ID        string     `json:"id"`
	TaskID    string     `json:"task_id"`
	AuthorID  string     `json:"author_id"`
	Body      string     `json:"body"`
	Mentions  []string   `json:"mentions"`
	EditedAt  *time.Time `json:"edited_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// Member mirrors services/project/internal/api/dto.go's memberResponse.
type Member struct {
	ProjectID string    `json:"project_id"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email,omitempty"`
	Role      string    `json:"role"`
	InvitedBy *string   `json:"invited_by,omitempty"`
	JoinedAt  time.Time `json:"joined_at"`
}

// ColumnDef mirrors services/boardregistry/internal/api/dto.go's columnDefDTO —
// the default columns a board type is created with.
type ColumnDef struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
	WIPLimit *int   `json:"wip_limit,omitempty"`
	Status   string `json:"status"`
}

// BoardTypeDef mirrors services/boardregistry/internal/api/dto.go's boardTypeResponse.
type BoardTypeDef struct {
	Type           string         `json:"type"`
	DisplayName    string         `json:"display_name"`
	Icon           string         `json:"icon"`
	DefaultColumns []ColumnDef    `json:"default_columns"`
	DefaultConfig  map[string]any `json:"default_config"`
	ConfigSchema   map[string]any `json:"config_schema"`
	Presentation   map[string]any `json:"presentation,omitempty"`
	BuiltIn        bool           `json:"built_in"`
	CreatedBy      *string        `json:"created_by,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}
