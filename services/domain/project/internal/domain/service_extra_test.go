package domain_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/teamboard/services/domain/project/internal/domain"
)

// ── Projects ──────────────────────────────────────────────────────────────────

func TestListProjectsForUser_HappyPath(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	_, err := svc.CreateProject(context.Background(), ownerID, "P1", "")
	require.NoError(t, err)
	_, err = svc.CreateProject(context.Background(), ownerID, "P2", "")
	require.NoError(t, err)

	list, err := svc.ListProjectsForUser(context.Background(), ownerID)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

// ── Members ───────────────────────────────────────────────────────────────────

func TestListMembers_HappyPath(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")
	seedKnownUser(t, repo, editorID, "e@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	_, err = svc.AddMember(context.Background(), proj.ID, ownerID, editorID, "", domain.RoleEditor)
	require.NoError(t, err)

	members, err := svc.ListMembers(context.Background(), proj.ID, ownerID)
	require.NoError(t, err)
	assert.Len(t, members, 2)
}

func TestUpdateMemberRole_HappyPath(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	secondOwnerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")
	seedKnownUser(t, repo, editorID, "e@x.com")
	seedKnownUser(t, repo, secondOwnerID, "o2@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	_, err = svc.AddMember(context.Background(), proj.ID, ownerID, editorID, "", domain.RoleEditor)
	require.NoError(t, err)

	// Promote editor to owner so we have two owners
	_, err = svc.UpdateMemberRole(context.Background(), proj.ID, ownerID, editorID, domain.RoleOwner)
	require.NoError(t, err)

	// Now demote original owner (two owners exist)
	updated, err := svc.UpdateMemberRole(context.Background(), proj.ID, editorID, ownerID, domain.RoleEditor)
	require.NoError(t, err)
	assert.Equal(t, domain.RoleEditor, updated.Role)
}

func TestRemoveMember_ByOwner(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	viewerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")
	seedKnownUser(t, repo, viewerID, "v@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	_, err = svc.AddMember(context.Background(), proj.ID, ownerID, viewerID, "", domain.RoleViewer)
	require.NoError(t, err)

	require.NoError(t, svc.RemoveMember(context.Background(), proj.ID, ownerID, viewerID))

	_, err = repo.GetMember(context.Background(), proj.ID, viewerID)
	assert.ErrorIs(t, err, domain.ErrMemberNotFound)
}

// ── Boards ────────────────────────────────────────────────────────────────────

func TestGetBoard_HappyPath(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	board, err := svc.CreateBoard(context.Background(), proj.ID, ownerID, domain.BoardInput{Name: "B", Type: domain.BoardTypeKanban})
	require.NoError(t, err)

	got, err := svc.GetBoard(context.Background(), board.ID, ownerID)
	require.NoError(t, err)
	assert.Equal(t, board.ID, got.ID)
}

func TestGetBoard_NotFound(t *testing.T) {
	svc, _, _ := newSvc(t)
	_, err := svc.GetBoard(context.Background(), uuid.New(), uuid.New())
	require.Error(t, err)
	var de *domain.Error
	require.True(t, errorAs(err, &de))
	assert.Equal(t, "board_not_found", de.Code)
}

func TestListBoards_HappyPath(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	_, err = svc.CreateBoard(context.Background(), proj.ID, ownerID, domain.BoardInput{Name: "B1", Type: domain.BoardTypeKanban})
	require.NoError(t, err)
	_, err = svc.CreateBoard(context.Background(), proj.ID, ownerID, domain.BoardInput{Name: "B2", Type: domain.BoardTypeScrum})
	require.NoError(t, err)

	boards, err := svc.ListBoards(context.Background(), proj.ID, ownerID)
	require.NoError(t, err)
	assert.Len(t, boards, 2)
}

func TestUpdateBoard_HappyPath(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	board, err := svc.CreateBoard(context.Background(), proj.ID, ownerID, domain.BoardInput{Name: "Old", Type: domain.BoardTypeKanban})
	require.NoError(t, err)

	newName := "New Name"
	updated, err := svc.UpdateBoard(context.Background(), board.ID, ownerID, domain.BoardPatch{Name: &newName})
	require.NoError(t, err)
	assert.Equal(t, "New Name", updated.Name)
}

// ── Columns ───────────────────────────────────────────────────────────────────

func TestListColumns_HappyPath(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	board, err := svc.CreateBoard(context.Background(), proj.ID, ownerID, domain.BoardInput{Name: "B", Type: domain.BoardTypeKanban})
	require.NoError(t, err)

	cols, err := svc.ListColumns(context.Background(), board.ID, ownerID)
	require.NoError(t, err)
	assert.Len(t, cols, 3, "kanban default columns")
}

func TestCreateColumn_HappyPath(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	board, err := svc.CreateBoard(context.Background(), proj.ID, ownerID, domain.BoardInput{
		Name:    "B",
		Type:    domain.BoardTypeKanban,
		Columns: []domain.BoardColumnInput{{Name: "Only", Position: 0}},
	})
	require.NoError(t, err)

	wip := 5
	col, err := svc.CreateColumn(context.Background(), board.ID, ownerID, domain.BoardColumnInput{
		Name:     "Extra",
		Position: 1,
		WIPLimit: &wip,
	})
	require.NoError(t, err)
	assert.Equal(t, "Extra", col.Name)
	assert.Equal(t, &wip, col.WIPLimit)
}

// ── Strategies ────────────────────────────────────────────────────────────────

func TestCalendarStrategy(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	board, err := svc.CreateBoard(context.Background(), proj.ID, ownerID, domain.BoardInput{
		Name: "Cal",
		Type: domain.BoardTypeCalendar,
	})
	require.NoError(t, err)
	assert.Empty(t, board.Columns, "calendar has no default columns")
}

func TestCalendarStrategy_InvalidConfig(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	_, err = svc.CreateBoard(context.Background(), proj.ID, ownerID, domain.BoardInput{
		Name:   "Cal",
		Type:   domain.BoardTypeCalendar,
		Config: map[string]any{"week_start": "wednesday"},
	})
	require.Error(t, err)
	var de *domain.Error
	require.True(t, errorAs(err, &de))
	assert.Equal(t, "validation_failed", de.Code)
}

func TestScrumStrategy_InvalidSprintLength(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	_, err = svc.CreateBoard(context.Background(), proj.ID, ownerID, domain.BoardInput{
		Name:   "Sprint",
		Type:   domain.BoardTypeScrum,
		Config: map[string]any{"sprint_length_days": float64(0)},
	})
	require.Error(t, err)
}

// ── Permissions ───────────────────────────────────────────────────────────────

func TestPermissionsForRole_AllRoles(t *testing.T) {
	viewerPerms := domain.PermissionsForRole(domain.RoleViewer)
	assert.Contains(t, viewerPerms, "project:read")
	assert.NotContains(t, viewerPerms, "project:delete")

	editorPerms := domain.PermissionsForRole(domain.RoleEditor)
	assert.Contains(t, editorPerms, "board:create")
	assert.NotContains(t, editorPerms, "member:invite")

	ownerPerms := domain.PermissionsForRole(domain.RoleOwner)
	assert.Contains(t, ownerPerms, "member:invite")
	assert.Contains(t, ownerPerms, "project:delete")

	assert.Nil(t, domain.PermissionsForRole("unknown"))
}

// ── helper ─────────────────────────────────────────────────────────────────

func errorAs(err error, target any) bool {
	if de, ok := target.(**domain.Error); ok {
		if e, ok := err.(*domain.Error); ok {
			*de = e
			return true
		}
	}
	return false
}
