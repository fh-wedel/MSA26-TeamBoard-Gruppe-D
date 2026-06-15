package domain

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/teamboard/services/project/internal/boardplugins"
)

type service struct {
	repo  Repository
	cache PermissionCache
}

// NewProjectService constructs the domain service.
func NewProjectService(repo Repository, cache PermissionCache) ProjectService {
	return &service{repo: repo, cache: cache}
}

var _ ProjectService = (*service)(nil)

// ── Permission helpers ────────────────────────────────────────────────────────

func (s *service) requirePermission(ctx context.Context, projectID, userID uuid.UUID, perm string) error {
	ps, err := s.GetPermissions(ctx, projectID, userID)
	if err != nil {
		return err
	}
	if !ps.IsMember {
		return ErrPermissionDenied
	}
	if !HasPermission(ps.Role, perm) {
		return ErrPermissionDenied
	}
	return nil
}

// GetPermissions implements UC-9: Redis-cached permission lookup.
func (s *service) GetPermissions(ctx context.Context, projectID, userID uuid.UUID) (*PermissionSet, error) {
	// 1. Cache lookup
	if ps, err := s.cache.Get(ctx, projectID, userID); err == nil && ps != nil {
		return ps, nil
	}

	// 2. Check project exists
	proj, err := s.repo.GetProject(ctx, projectID)
	projectExists := err == nil && proj != nil

	// 3. DB lookup
	role, err := s.repo.GetMemberRole(ctx, projectID, userID)
	if err != nil || role == "" {
		ps := &PermissionSet{
			Permissions:   []string{},
			ProjectExists: projectExists,
			IsMember:      false,
		}
		_ = s.cache.Set(ctx, projectID, userID, ps)
		return ps, nil
	}

	ps := &PermissionSet{
		Role:          role,
		Permissions:   PermissionsForRole(role),
		ProjectExists: true,
		IsMember:      true,
	}
	_ = s.cache.Set(ctx, projectID, userID, ps)
	return ps, nil
}

// ── Projects (UC-1 to UC-3, UC-8) ────────────────────────────────────────────

func (s *service) CreateProject(ctx context.Context, ownerID uuid.UUID, name, description string) (*Project, error) {
	if name == "" || len(name) > 200 {
		return nil, ErrValidation
	}

	var proj *Project
	err := s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		proj, txErr = tx.CreateProject(ctx, uuid.New(), ownerID, name, description)
		if txErr != nil {
			return txErr
		}

		// Owner membership
		if _, txErr = tx.AddMember(ctx, proj.ID, ownerID, RoleOwner, nil); txErr != nil {
			return txErr
		}

		payload, _ := json.Marshal(map[string]any{
			"project_id": proj.ID,
			"name":       proj.Name,
			"owner_id":   proj.OwnerID,
			"created_at": proj.CreatedAt,
		})
		if txErr = tx.InsertOutboxEvent(ctx, uuid.New(), proj.ID, "project.created", payload); txErr != nil {
			return txErr
		}

		memberPayload, _ := json.Marshal(map[string]any{
			"project_id": proj.ID,
			"user_id":    ownerID,
			"role":       RoleOwner,
		})
		return tx.InsertOutboxEvent(ctx, uuid.New(), proj.ID, "project.member.added", memberPayload)
	})
	if err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "project created", "project_id", proj.ID, "owner_id", ownerID)
	return proj, nil
}

func (s *service) GetProject(ctx context.Context, id, requester uuid.UUID) (*Project, error) {
	if err := s.requirePermission(ctx, id, requester, "project:read"); err != nil {
		return nil, err
	}
	proj, err := s.repo.GetProject(ctx, id)
	if err != nil {
		return nil, ErrProjectNotFound
	}
	return proj, nil
}

func (s *service) ListProjectsForUser(ctx context.Context, userID uuid.UUID) ([]*Project, error) {
	return s.repo.ListProjectsForUser(ctx, userID)
}

func (s *service) UpdateProject(ctx context.Context, id, requester uuid.UUID, patch ProjectPatch) (*Project, error) {
	if err := s.requirePermission(ctx, id, requester, "project:update"); err != nil {
		return nil, err
	}

	proj, err := s.repo.UpdateProject(ctx, id, patch.Name, patch.Description)
	if err != nil {
		return nil, ErrProjectNotFound
	}

	payload, _ := json.Marshal(map[string]any{"project_id": id})
	_ = s.repo.InsertOutboxEvent(ctx, uuid.New(), id, "project.updated", payload)
	return proj, nil
}

func (s *service) DeleteProject(ctx context.Context, id, requester uuid.UUID) error {
	if err := s.requirePermission(ctx, id, requester, "project:delete"); err != nil {
		return err
	}

	if err := s.repo.SoftDeleteProject(ctx, id); err != nil {
		return ErrProjectNotFound
	}

	payload, _ := json.Marshal(map[string]any{"project_id": id})
	_ = s.repo.InsertOutboxEvent(ctx, uuid.New(), id, "project.deleted", payload)
	return nil
}

// ── Members (UC-4 to UC-6) ────────────────────────────────────────────────────

func (s *service) AddMember(ctx context.Context, projectID, requester uuid.UUID, userID uuid.UUID, email string, role Role) (*ProjectMember, error) {
	if err := s.requirePermission(ctx, projectID, requester, "member:invite"); err != nil {
		return nil, err
	}
	if !ValidRole(role) {
		return nil, ErrInvalidRole
	}

	// Resolve user: prefer userID, fall back to email lookup
	if userID == uuid.Nil && email != "" {
		ku, err := s.repo.GetKnownUserByEmail(ctx, email)
		if err != nil {
			return nil, ErrUnknownUser
		}
		userID = ku.ID
	} else if userID != uuid.Nil {
		if _, err := s.repo.GetKnownUserByID(ctx, userID); err != nil {
			return nil, ErrUnknownUser
		}
	} else {
		return nil, &Error{Code: "validation_failed", Message: "user_id or email required"}
	}

	// Check not already a member
	if _, err := s.repo.GetMember(ctx, projectID, userID); err == nil {
		return nil, ErrAlreadyMember
	}

	var member *ProjectMember
	err := s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		member, txErr = tx.AddMember(ctx, projectID, userID, role, &requester)
		if txErr != nil {
			return txErr
		}
		payload, _ := json.Marshal(map[string]any{
			"project_id": projectID,
			"user_id":    userID,
			"role":       role,
			"invited_by": requester,
		})
		return tx.InsertOutboxEvent(ctx, uuid.New(), projectID, "project.member.added", payload)
	})
	if err != nil {
		return nil, err
	}

	// Invalidate cache after commit
	_ = s.cache.Delete(ctx, projectID, userID)
	return member, nil
}

func (s *service) UpdateMemberRole(ctx context.Context, projectID, requester, userID uuid.UUID, newRole Role) (*ProjectMember, error) {
	if err := s.requirePermission(ctx, projectID, requester, "member:update"); err != nil {
		return nil, err
	}
	if !ValidRole(newRole) {
		return nil, ErrInvalidRole
	}

	current, err := s.repo.GetMember(ctx, projectID, userID)
	if err != nil {
		return nil, ErrMemberNotFound
	}

	// Last-owner protection
	if current.Role == RoleOwner && newRole != RoleOwner {
		count, err := s.repo.CountOwners(ctx, projectID)
		if err != nil || count <= 1 {
			return nil, ErrLastOwner
		}
	}

	var updated *ProjectMember
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		updated, txErr = tx.UpdateMemberRole(ctx, projectID, userID, newRole)
		if txErr != nil {
			return txErr
		}
		payload, _ := json.Marshal(map[string]any{
			"project_id": projectID,
			"user_id":    userID,
			"old_role":   current.Role,
			"new_role":   newRole,
		})
		return tx.InsertOutboxEvent(ctx, uuid.New(), projectID, "project.member.role_changed", payload)
	})
	if err != nil {
		return nil, err
	}

	_ = s.cache.Delete(ctx, projectID, userID)
	return updated, nil
}

func (s *service) RemoveMember(ctx context.Context, projectID, requester, userID uuid.UUID) error {
	// Allow self-leave OR owner removing anyone
	isSelf := requester == userID
	if !isSelf {
		if err := s.requirePermission(ctx, projectID, requester, "member:remove"); err != nil {
			return err
		}
	} else {
		// Even for self-leave, confirm requester is a member
		if _, err := s.repo.GetMember(ctx, projectID, requester); err != nil {
			return ErrMemberNotFound
		}
	}

	target, err := s.repo.GetMember(ctx, projectID, userID)
	if err != nil {
		return ErrMemberNotFound
	}

	// Last-owner protection
	if target.Role == RoleOwner {
		count, err := s.repo.CountOwners(ctx, projectID)
		if err != nil || count <= 1 {
			return ErrLastOwner
		}
	}

	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		if txErr := tx.RemoveMember(ctx, projectID, userID); txErr != nil {
			return txErr
		}
		payload, _ := json.Marshal(map[string]any{"project_id": projectID, "user_id": userID})
		return tx.InsertOutboxEvent(ctx, uuid.New(), projectID, "project.member.removed", payload)
	})
	if err != nil {
		return err
	}

	_ = s.cache.Delete(ctx, projectID, userID)
	return nil
}

func (s *service) ListMembers(ctx context.Context, projectID, requester uuid.UUID) ([]*ProjectMember, error) {
	if err := s.requirePermission(ctx, projectID, requester, "member:read"); err != nil {
		return nil, err
	}
	return s.repo.ListMembers(ctx, projectID)
}

// ── Boards (UC-7) ─────────────────────────────────────────────────────────────

func (s *service) CreateBoard(ctx context.Context, projectID, requester uuid.UUID, input BoardInput) (*Board, error) {
	if err := s.requirePermission(ctx, projectID, requester, "board:create"); err != nil {
		return nil, err
	}

	plugin, ok := boardplugins.Get(string(input.Type))
	if !ok {
		return nil, ErrInvalidBoardType
	}

	// Merge config with defaults
	cfg := plugin.DefaultConfig()
	for k, v := range input.Config {
		cfg[k] = v
	}
	if err := plugin.ValidateConfig(cfg); err != nil {
		return nil, ErrValidation
	}

	// Use default columns if none specified; map plugin type to domain type
	cols := input.Columns
	if len(cols) == 0 {
		for _, c := range plugin.DefaultColumns() {
			cols = append(cols, BoardColumnInput{Name: c.Name, Position: c.Position, WIPLimit: c.WIPLimit})
		}
	}

	cfgJSON, _ := json.Marshal(cfg)
	pos, err := s.repo.GetMaxBoardPosition(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get board position: %w", err)
	}

	var board *Board
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		board, txErr = tx.CreateBoard(ctx, uuid.New(), projectID, input.Name, input.Type, pos+1, cfgJSON, requester)
		if txErr != nil {
			return txErr
		}

		for _, col := range cols {
			c, txErr := tx.CreateColumn(ctx, uuid.New(), board.ID, col.Name, col.Position, col.WIPLimit)
			if txErr != nil {
				return txErr
			}
			board.Columns = append(board.Columns, *c)
		}

		type colEntry struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Position int    `json:"position"`
		}
		colEntries := make([]colEntry, len(board.Columns))
		for i, c := range board.Columns {
			colEntries[i] = colEntry{ID: c.ID.String(), Name: c.Name, Position: c.Position}
		}
		payload, _ := json.Marshal(map[string]any{
			"board_id":   board.ID,
			"project_id": projectID,
			"name":       board.Name,
			"type":       board.Type,
			"created_by": requester,
			"columns":    colEntries,
		})
		return tx.InsertOutboxEvent(ctx, uuid.New(), board.ID, "board.created", payload)
	})
	if err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "board created", "board_id", board.ID, "project_id", projectID)
	return board, nil
}

func (s *service) GetBoard(ctx context.Context, boardID, requester uuid.UUID) (*Board, error) {
	board, err := s.repo.GetBoard(ctx, boardID)
	if err != nil {
		return nil, ErrBoardNotFound
	}
	if err := s.requirePermission(ctx, board.ProjectID, requester, "board:read"); err != nil {
		return nil, err
	}
	cols, _ := s.repo.ListColumnsByBoard(ctx, boardID)
	board.Columns = cols
	return board, nil
}

func (s *service) ListBoards(ctx context.Context, projectID, requester uuid.UUID) ([]*Board, error) {
	if err := s.requirePermission(ctx, projectID, requester, "board:read"); err != nil {
		return nil, err
	}
	return s.repo.ListBoardsByProject(ctx, projectID)
}

func (s *service) UpdateBoard(ctx context.Context, boardID, requester uuid.UUID, patch BoardPatch) (*Board, error) {
	board, err := s.repo.GetBoard(ctx, boardID)
	if err != nil {
		return nil, ErrBoardNotFound
	}
	if err := s.requirePermission(ctx, board.ProjectID, requester, "board:update"); err != nil {
		return nil, err
	}

	var cfgJSON []byte
	if patch.Config != nil {
		cfgJSON, _ = json.Marshal(patch.Config)
	}

	updated, err := s.repo.UpdateBoard(ctx, boardID, patch.Name, cfgJSON)
	if err != nil {
		return nil, ErrBoardNotFound
	}

	payload, _ := json.Marshal(map[string]any{"board_id": boardID, "project_id": board.ProjectID})
	_ = s.repo.InsertOutboxEvent(ctx, uuid.New(), boardID, "board.updated", payload)
	return updated, nil
}

func (s *service) DeleteBoard(ctx context.Context, boardID, requester uuid.UUID) error {
	board, err := s.repo.GetBoard(ctx, boardID)
	if err != nil {
		return ErrBoardNotFound
	}
	if err := s.requirePermission(ctx, board.ProjectID, requester, "board:delete"); err != nil {
		return err
	}

	if err := s.repo.SoftDeleteBoard(ctx, boardID); err != nil {
		return ErrBoardNotFound
	}

	payload, _ := json.Marshal(map[string]any{"board_id": boardID, "project_id": board.ProjectID})
	_ = s.repo.InsertOutboxEvent(ctx, uuid.New(), boardID, "board.deleted", payload)
	return nil
}

// ── Columns ───────────────────────────────────────────────────────────────────

func (s *service) ListColumns(ctx context.Context, boardID, requester uuid.UUID) ([]BoardColumn, error) {
	board, err := s.repo.GetBoard(ctx, boardID)
	if err != nil {
		return nil, ErrBoardNotFound
	}
	if err := s.requirePermission(ctx, board.ProjectID, requester, "board:read"); err != nil {
		return nil, err
	}
	return s.repo.ListColumnsByBoard(ctx, boardID)
}

func (s *service) CreateColumn(ctx context.Context, boardID, requester uuid.UUID, input BoardColumnInput) (*BoardColumn, error) {
	board, err := s.repo.GetBoard(ctx, boardID)
	if err != nil {
		return nil, ErrBoardNotFound
	}
	if err := s.requirePermission(ctx, board.ProjectID, requester, "board:update"); err != nil {
		return nil, err
	}

	var col *BoardColumn
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		col, txErr = tx.CreateColumn(ctx, uuid.New(), boardID, input.Name, input.Position, input.WIPLimit)
		if txErr != nil {
			return txErr
		}
		payload, _ := json.Marshal(map[string]any{
			"column_id": col.ID,
			"board_id":  boardID,
			"name":      col.Name,
			"position":  col.Position,
		})
		return tx.InsertOutboxEvent(ctx, uuid.New(), col.ID, "column.created", payload)
	})
	if err != nil {
		return nil, err
	}
	return col, nil
}

// ── Event helpers ─────────────────────────────────────────────────────────────

// HandleUserRegistered upserts the user into known_users (idempotent).
func (s *service) HandleUserRegistered(ctx context.Context, eventID string, userID uuid.UUID, email string, createdAt interface{}) error {
	return nil // wired through the event consumer, not the ProjectService interface
}

// invalidateMemberCache is a convenience wrapper.
func (s *service) invalidateMemberCache(ctx context.Context, projectID, userID uuid.UUID) {
	if err := s.cache.Delete(ctx, projectID, userID); err != nil {
		slog.WarnContext(ctx, "cache invalidation failed", "project_id", projectID, "user_id", userID, "error", err)
	}
}

// ── Invitations ───────────────────────────────────────────────────────────────

func (s *service) CreateInvitation(ctx context.Context, projectID, requester uuid.UUID, email string, role Role) (*Invitation, error) {
	if err := s.requirePermission(ctx, projectID, requester, "member:invite"); err != nil {
		return nil, err
	}
	if !ValidRole(role) {
		return nil, ErrInvalidRole
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, ErrValidation
	}

	// Invitee must be a known user
	invitee, err := s.repo.GetKnownUserByEmail(ctx, email)
	if err != nil || invitee == nil || invitee.DeletedAt != nil {
		return nil, ErrUnknownUser
	}

	// Cannot invite existing members
	if _, err := s.repo.GetMember(ctx, projectID, invitee.ID); err == nil {
		return nil, ErrAlreadyMember
	}

	// Cannot double-invite
	if existing, _ := s.repo.GetPendingInvitationByEmailAndProject(ctx, email, projectID); existing != nil {
		return nil, ErrAlreadyInvited
	}

	// Generate token
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(buf)

	var inv *Invitation
	requesterUser, _ := s.repo.GetKnownUserByID(ctx, requester)
	requesterEmail := ""
	if requesterUser != nil {
		requesterEmail = requesterUser.Email
	}
	project, _ := s.repo.GetProject(ctx, projectID)
	projectName := ""
	if project != nil {
		projectName = project.Name
	}

	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		var txErr error
		inv, txErr = tx.CreateInvitation(ctx, uuid.New(), projectID, email, string(role), token, requester)
		if txErr != nil {
			return txErr
		}
		payload, _ := json.Marshal(map[string]any{
			"invitation_id":    inv.ID,
			"project_id":       projectID,
			"project_name":     projectName,
			"invitee_email":    email,
			"invitee_id":       invitee.ID,
			"invited_by":       requester,
			"invited_by_email": requesterEmail,
			"role":             string(role),
			"token":            token,
			"expires_at":       inv.ExpiresAt,
		})
		return tx.InsertOutboxEvent(ctx, uuid.New(), projectID, "project.invitation.sent", payload)
	})
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func (s *service) GetInvitationByToken(ctx context.Context, token string) (*Invitation, error) {
	inv, err := s.repo.GetInvitationByToken(ctx, token)
	if err != nil {
		return nil, ErrInvitationNotFound
	}
	return inv, nil
}

func (s *service) AcceptInvitation(ctx context.Context, token string, acceptorID uuid.UUID, acceptorEmail string) (*ProjectMember, error) {
	inv, err := s.repo.GetInvitationByToken(ctx, token)
	if err != nil {
		return nil, ErrInvitationNotFound
	}
	if strings.ToLower(inv.InviteeEmail) != strings.ToLower(acceptorEmail) {
		return nil, ErrInvitationMismatch
	}
	if inv.Status != InvitationStatusPending {
		return nil, ErrInvitationNotPending
	}
	if inv.IsExpired() {
		return nil, ErrInvitationExpired
	}

	project, _ := s.repo.GetProject(ctx, inv.ProjectID)
	projectName := ""
	if project != nil {
		projectName = project.Name
	}

	var member *ProjectMember
	err = s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		// Mark invitation as accepted
		if txErr := tx.UpdateInvitationStatus(ctx, inv.ID, InvitationStatusAccepted); txErr != nil {
			return txErr
		}
		// Add as member
		var txErr error
		member, txErr = tx.AddMember(ctx, inv.ProjectID, acceptorID, inv.Role, &inv.InvitedBy)
		if txErr != nil {
			return txErr
		}
		payload, _ := json.Marshal(map[string]any{
			"invitation_id":  inv.ID,
			"project_id":     inv.ProjectID,
			"project_name":   projectName,
			"invitee_id":     acceptorID,
			"invitee_email":  acceptorEmail,
			"invited_by":     inv.InvitedBy,
		})
		if txErr = tx.InsertOutboxEvent(ctx, uuid.New(), inv.ProjectID, "project.invitation.accepted", payload); txErr != nil {
			return txErr
		}
		// Also fire member.added for other consumers
		memberPayload, _ := json.Marshal(map[string]any{
			"project_id": inv.ProjectID,
			"user_id":    acceptorID,
			"added_by":   inv.InvitedBy,
		})
		return tx.InsertOutboxEvent(ctx, uuid.New(), inv.ProjectID, "project.member.added", memberPayload)
	})
	if err != nil {
		return nil, err
	}
	_ = s.cache.Delete(ctx, inv.ProjectID, acceptorID)
	return member, nil
}

func (s *service) DeclineInvitation(ctx context.Context, token string, declinerID uuid.UUID, declinerEmail string) error {
	inv, err := s.repo.GetInvitationByToken(ctx, token)
	if err != nil {
		return ErrInvitationNotFound
	}
	if strings.ToLower(inv.InviteeEmail) != strings.ToLower(declinerEmail) {
		return ErrInvitationMismatch
	}
	if inv.Status != InvitationStatusPending {
		return ErrInvitationNotPending
	}

	project, _ := s.repo.GetProject(ctx, inv.ProjectID)
	projectName := ""
	if project != nil {
		projectName = project.Name
	}

	return s.repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		if txErr := tx.UpdateInvitationStatus(ctx, inv.ID, InvitationStatusDeclined); txErr != nil {
			return txErr
		}
		payload, _ := json.Marshal(map[string]any{
			"invitation_id": inv.ID,
			"project_id":    inv.ProjectID,
			"project_name":  projectName,
			"invitee_id":    declinerID,
			"invitee_email": declinerEmail,
			"invited_by":    inv.InvitedBy,
		})
		return tx.InsertOutboxEvent(ctx, uuid.New(), inv.ProjectID, "project.invitation.declined", payload)
	})
}

func (s *service) ListInvitations(ctx context.Context, projectID, requester uuid.UUID) ([]*Invitation, error) {
	if err := s.requirePermission(ctx, projectID, requester, "member:read"); err != nil {
		return nil, err
	}
	return s.repo.GetInvitationsByProject(ctx, projectID)
}

// unwrap helpers for errors
var _ = errors.Is
var _ = time.Now
