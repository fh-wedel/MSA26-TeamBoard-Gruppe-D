package api

import (
	"time"

	"github.com/google/uuid"
	"github.com/teamboard/services/project/internal/domain"
)

// ── Requests ──────────────────────────────────────────────────────────────────

type createProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type updateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type addMemberRequest struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

type updateMemberRoleRequest struct {
	Role string `json:"role"`
}

type createBoardRequest struct {
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	Config  map[string]any `json:"config"`
	Columns []columnInput  `json:"columns"`
}

type updateBoardRequest struct {
	Name   *string        `json:"name"`
	Config map[string]any `json:"config"`
}

type columnInput struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
	WIPLimit *int   `json:"wip_limit"`
}

type createColumnRequest struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
	WIPLimit *int   `json:"wip_limit"`
}

// ── Responses ─────────────────────────────────────────────────────────────────

type projectResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	OwnerID     uuid.UUID `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type memberResponse struct {
	ProjectID uuid.UUID  `json:"project_id"`
	UserID    uuid.UUID  `json:"user_id"`
	Email     string     `json:"email,omitempty"`
	Role      string     `json:"role"`
	InvitedBy *uuid.UUID `json:"invited_by,omitempty"`
	JoinedAt  time.Time  `json:"joined_at"`
}

type boardResponse struct {
	ID        uuid.UUID      `json:"id"`
	ProjectID uuid.UUID      `json:"project_id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Position  int            `json:"position"`
	Config    map[string]any `json:"config"`
	CreatedBy uuid.UUID      `json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Columns   []columnResponse `json:"columns,omitempty"`
}

type columnResponse struct {
	ID       uuid.UUID `json:"id"`
	BoardID  uuid.UUID `json:"board_id"`
	Name     string    `json:"name"`
	Position int       `json:"position"`
	WIPLimit *int      `json:"wip_limit,omitempty"`
}

type permissionResponse struct {
	Role          string   `json:"role,omitempty"`
	Permissions   []string `json:"permissions"`
	ProjectExists bool     `json:"project_exists"`
	IsMember      bool     `json:"is_member"`
}

// ── Mapping ───────────────────────────────────────────────────────────────────

func mapProjectResponse(p *domain.Project) projectResponse {
	return projectResponse{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		OwnerID:     p.OwnerID,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func mapMemberResponse(m *domain.ProjectMember) memberResponse {
	return memberResponse{
		ProjectID: m.ProjectID,
		UserID:    m.UserID,
		Email:     m.Email,
		Role:      string(m.Role),
		InvitedBy: m.InvitedBy,
		JoinedAt:  m.JoinedAt,
	}
}

func mapBoardResponse(b *domain.Board) boardResponse {
	cols := make([]columnResponse, len(b.Columns))
	for i, c := range b.Columns {
		cols[i] = mapColumnResponse(c)
	}
	return boardResponse{
		ID:        b.ID,
		ProjectID: b.ProjectID,
		Name:      b.Name,
		Type:      string(b.Type),
		Position:  b.Position,
		Config:    b.Config,
		CreatedBy: b.CreatedBy,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
		Columns:   cols,
	}
}

func mapColumnResponse(c domain.BoardColumn) columnResponse {
	return columnResponse{
		ID:       c.ID,
		BoardID:  c.BoardID,
		Name:     c.Name,
		Position: c.Position,
		WIPLimit: c.WIPLimit,
	}
}

func mapPermissionResponse(ps *domain.PermissionSet) permissionResponse {
	return permissionResponse{
		Role:          string(ps.Role),
		Permissions:   ps.Permissions,
		ProjectExists: ps.ProjectExists,
		IsMember:      ps.IsMember,
	}
}
