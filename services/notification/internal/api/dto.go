package api

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/teamboard/services/notification/internal/domain"
)

type notificationDTO struct {
	ID        uuid.UUID      `json:"id"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	ProjectID *uuid.UUID     `json:"project_id,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	ReadAt    *time.Time     `json:"read_at"`
}

type notificationListDTO struct {
	Data       []notificationDTO `json:"data"`
	Pagination *paginationDTO    `json:"pagination,omitempty"`
}

type paginationDTO struct {
	NextCursor *string `json:"next_cursor,omitempty"`
}

func mapNotification(n *domain.Notification) notificationDTO {
	return notificationDTO{
		ID:        n.ID,
		Type:      string(n.Type),
		Payload:   n.Payload,
		ProjectID: n.ProjectID,
		CreatedAt: n.CreatedAt,
		ReadAt:    n.ReadAt,
	}
}

func encodeCursor(c *domain.PageCursor) *string {
	if c == nil {
		return nil
	}
	raw, _ := json.Marshal(c)
	s := base64.StdEncoding.EncodeToString(raw)
	return &s
}
