package domain_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/teamboard/services/project/internal/domain"
)

// ── Fake repository ───────────────────────────────────────────────────────────

type fakeRepo struct {
	mu         sync.Mutex
	projects   map[uuid.UUID]*domain.Project
	members    map[string]*domain.ProjectMember // key: "projectID:userID"
	boards     map[uuid.UUID]*domain.Board
	columns    map[uuid.UUID]*domain.BoardColumn
	knownUsers  map[uuid.UUID]*domain.KnownUser
	emailIndex  map[string]*domain.KnownUser
	processed   map[string]bool
	invitations map[string]*domain.Invitation // key: token
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		projects:   make(map[uuid.UUID]*domain.Project),
		members:    make(map[string]*domain.ProjectMember),
		boards:     make(map[uuid.UUID]*domain.Board),
		columns:    make(map[uuid.UUID]*domain.BoardColumn),
		knownUsers:  make(map[uuid.UUID]*domain.KnownUser),
		emailIndex:  make(map[string]*domain.KnownUser),
		processed:   make(map[string]bool),
		invitations: make(map[string]*domain.Invitation),
	}
}

func (r *fakeRepo) CreateInvitation(_ context.Context, id, projectID uuid.UUID, email, role, token string, invitedBy uuid.UUID) (*domain.Invitation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv := &domain.Invitation{
		ID: id, ProjectID: projectID, InviteeEmail: email, Role: domain.Role(role),
		Token: token, InvitedBy: invitedBy, Status: domain.InvitationStatusPending,
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(72 * time.Hour),
	}
	r.invitations[token] = inv
	return inv, nil
}

func (r *fakeRepo) GetInvitationByToken(_ context.Context, token string) (*domain.Invitation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv, ok := r.invitations[token]
	if !ok {
		return nil, domain.ErrInvitationNotFound
	}
	return inv, nil
}

func (r *fakeRepo) GetPendingInvitationByEmailAndProject(_ context.Context, email string, projectID uuid.UUID) (*domain.Invitation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, inv := range r.invitations {
		if inv.InviteeEmail == email && inv.ProjectID == projectID && inv.Status == domain.InvitationStatusPending {
			return inv, nil
		}
	}
	return nil, nil
}

func (r *fakeRepo) GetInvitationsByProject(_ context.Context, projectID uuid.UUID) ([]*domain.Invitation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Invitation
	for _, inv := range r.invitations {
		if inv.ProjectID == projectID {
			out = append(out, inv)
		}
	}
	return out, nil
}

func (r *fakeRepo) UpdateInvitationStatus(_ context.Context, id uuid.UUID, status domain.InvitationStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, inv := range r.invitations {
		if inv.ID == id {
			inv.Status = status
			return nil
		}
	}
	return domain.ErrInvitationNotFound
}

func memberKey(projectID, userID uuid.UUID) string {
	return projectID.String() + ":" + userID.String()
}

func (r *fakeRepo) WithTransaction(ctx context.Context, fn func(context.Context, domain.Repository) error) error {
	return fn(ctx, r)
}

func (r *fakeRepo) CreateProject(_ context.Context, id, ownerID uuid.UUID, name, description string) (*domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := &domain.Project{ID: id, Name: name, Description: description, OwnerID: ownerID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	r.projects[id] = p
	return p, nil
}

func (r *fakeRepo) GetProject(_ context.Context, id uuid.UUID) (*domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.projects[id]
	if !ok || p.DeletedAt != nil {
		return nil, domain.ErrProjectNotFound
	}
	return p, nil
}

func (r *fakeRepo) ListProjectsForUser(_ context.Context, userID uuid.UUID) ([]*domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Project
	for _, m := range r.members {
		if m.UserID == userID {
			if p, ok := r.projects[m.ProjectID]; ok && p.DeletedAt == nil {
				out = append(out, p)
			}
		}
	}
	return out, nil
}

func (r *fakeRepo) UpdateProject(_ context.Context, id uuid.UUID, name, description *string) (*domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.projects[id]
	if !ok || p.DeletedAt != nil {
		return nil, domain.ErrProjectNotFound
	}
	if name != nil {
		p.Name = *name
	}
	if description != nil {
		p.Description = *description
	}
	return p, nil
}

func (r *fakeRepo) SoftDeleteProject(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.projects[id]
	if !ok || p.DeletedAt != nil {
		return domain.ErrProjectNotFound
	}
	now := time.Now()
	p.DeletedAt = &now
	return nil
}

func (r *fakeRepo) AddMember(_ context.Context, projectID, userID uuid.UUID, role domain.Role, invitedBy *uuid.UUID) (*domain.ProjectMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := &domain.ProjectMember{ProjectID: projectID, UserID: userID, Role: role, InvitedBy: invitedBy, JoinedAt: time.Now()}
	r.members[memberKey(projectID, userID)] = m
	return m, nil
}

func (r *fakeRepo) GetMember(_ context.Context, projectID, userID uuid.UUID) (*domain.ProjectMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.members[memberKey(projectID, userID)]
	if !ok {
		return nil, domain.ErrMemberNotFound
	}
	return m, nil
}

func (r *fakeRepo) ListMembers(_ context.Context, projectID uuid.UUID) ([]*domain.ProjectMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.ProjectMember
	for _, m := range r.members {
		if m.ProjectID == projectID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *fakeRepo) UpdateMemberRole(_ context.Context, projectID, userID uuid.UUID, role domain.Role) (*domain.ProjectMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.members[memberKey(projectID, userID)]
	if !ok {
		return nil, domain.ErrMemberNotFound
	}
	m.Role = role
	return m, nil
}

func (r *fakeRepo) RemoveMember(_ context.Context, projectID, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.members, memberKey(projectID, userID))
	return nil
}

func (r *fakeRepo) CountOwners(_ context.Context, projectID uuid.UUID) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, m := range r.members {
		if m.ProjectID == projectID && m.Role == domain.RoleOwner {
			n++
		}
	}
	return n, nil
}

func (r *fakeRepo) GetMemberRole(_ context.Context, projectID, userID uuid.UUID) (domain.Role, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.members[memberKey(projectID, userID)]
	if !ok {
		return "", domain.ErrMemberNotFound
	}
	return m.Role, nil
}

func (r *fakeRepo) RemoveAllMembershipsOfUser(_ context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []uuid.UUID
	for k, m := range r.members {
		if m.UserID == userID {
			ids = append(ids, m.ProjectID)
			delete(r.members, k)
		}
	}
	return ids, nil
}

func (r *fakeRepo) CreateBoard(_ context.Context, id, projectID uuid.UUID, name string, bType domain.BoardType, position int, config []byte, createdBy uuid.UUID) (*domain.Board, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := &domain.Board{ID: id, ProjectID: projectID, Name: name, Type: bType, Position: position, CreatedBy: createdBy, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	r.boards[id] = b
	return b, nil
}

func (r *fakeRepo) GetBoard(_ context.Context, id uuid.UUID) (*domain.Board, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.boards[id]
	if !ok || b.DeletedAt != nil {
		return nil, domain.ErrBoardNotFound
	}
	return b, nil
}

func (r *fakeRepo) ListBoardsByProject(_ context.Context, projectID uuid.UUID) ([]*domain.Board, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Board
	for _, b := range r.boards {
		if b.ProjectID == projectID && b.DeletedAt == nil {
			out = append(out, b)
		}
	}
	return out, nil
}

func (r *fakeRepo) UpdateBoard(_ context.Context, id uuid.UUID, name *string, config []byte) (*domain.Board, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.boards[id]
	if !ok || b.DeletedAt != nil {
		return nil, domain.ErrBoardNotFound
	}
	if name != nil {
		b.Name = *name
	}
	return b, nil
}

func (r *fakeRepo) SoftDeleteBoard(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.boards[id]
	if !ok || b.DeletedAt != nil {
		return domain.ErrBoardNotFound
	}
	now := time.Now()
	b.DeletedAt = &now
	return nil
}

func (r *fakeRepo) GetMaxBoardPosition(_ context.Context, projectID uuid.UUID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	max := -1
	for _, b := range r.boards {
		if b.ProjectID == projectID && b.DeletedAt == nil && b.Position > max {
			max = b.Position
		}
	}
	return max, nil
}

func (r *fakeRepo) CreateColumn(_ context.Context, id, boardID uuid.UUID, name string, position int, wipLimit *int, status string) (*domain.BoardColumn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := &domain.BoardColumn{ID: id, BoardID: boardID, Name: name, Position: position, WIPLimit: wipLimit, Status: status}
	r.columns[id] = c
	return c, nil
}

func (r *fakeRepo) ListColumnsByBoard(_ context.Context, boardID uuid.UUID) ([]domain.BoardColumn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.BoardColumn
	for _, c := range r.columns {
		if c.BoardID == boardID {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (r *fakeRepo) UpsertKnownUser(_ context.Context, id uuid.UUID, email string, createdAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u := &domain.KnownUser{ID: id, Email: email, CreatedAt: createdAt}
	r.knownUsers[id] = u
	r.emailIndex[email] = u
	return nil
}

func (r *fakeRepo) GetKnownUserByEmail(_ context.Context, email string) (*domain.KnownUser, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.emailIndex[email]
	if !ok || u.DeletedAt != nil {
		return nil, domain.ErrUnknownUser
	}
	return u, nil
}

func (r *fakeRepo) GetKnownUserByID(_ context.Context, id uuid.UUID) (*domain.KnownUser, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.knownUsers[id]
	if !ok || u.DeletedAt != nil {
		return nil, domain.ErrUnknownUser
	}
	return u, nil
}

func (r *fakeRepo) MarkKnownUserDeleted(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.knownUsers[id]; ok {
		now := time.Now()
		u.DeletedAt = &now
	}
	return nil
}

func (r *fakeRepo) InsertOutboxEvent(_ context.Context, _, _ uuid.UUID, _ string, _ []byte) error {
	return nil
}

func (r *fakeRepo) GetUnpublishedEvents(_ context.Context, _ int32) ([]*domain.OutboxEvent, error) {
	return nil, nil
}

func (r *fakeRepo) MarkEventPublished(_ context.Context, _ uuid.UUID) error { return nil }

func (r *fakeRepo) WasEventProcessed(_ context.Context, eventID string) (bool, error) {
	return r.processed[eventID], nil
}

func (r *fakeRepo) MarkEventProcessed(_ context.Context, eventID string) error {
	r.processed[eventID] = true
	return nil
}

// ── Fake cache ────────────────────────────────────────────────────────────────

type fakeCache struct {
	mu    sync.Mutex
	store map[string]*domain.PermissionSet
}

func newFakeCache() *fakeCache {
	return &fakeCache{store: make(map[string]*domain.PermissionSet)}
}

func cacheKey(projectID, userID uuid.UUID) string {
	return projectID.String() + ":" + userID.String()
}

func (c *fakeCache) Get(_ context.Context, projectID, userID uuid.UUID) (*domain.PermissionSet, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.store[cacheKey(projectID, userID)], nil
}

func (c *fakeCache) Set(_ context.Context, projectID, userID uuid.UUID, ps *domain.PermissionSet) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[cacheKey(projectID, userID)] = ps
	return nil
}

func (c *fakeCache) Delete(_ context.Context, projectID, userID uuid.UUID) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, cacheKey(projectID, userID))
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func seedKnownUser(t *testing.T, repo *fakeRepo, userID uuid.UUID, email string) {
	t.Helper()
	require.NoError(t, repo.UpsertKnownUser(context.Background(), userID, email, time.Now()))
}

// fakeBoardTypeRegistry serves the built-in board-type definitions for tests.
type fakeBoardTypeRegistry struct{}

func intPtr(i int) *int { return &i }

func (fakeBoardTypeRegistry) GetType(_ context.Context, typeID string) (*domain.BoardTypeDef, error) {
	switch typeID {
	case "kanban":
		return &domain.BoardTypeDef{
			Type: "kanban", DisplayName: "Kanban Board", Icon: "📋",
			DefaultColumns: []domain.BoardTypeColumn{
				{Name: "To Do", Position: 0, Status: "open"},
				{Name: "In Progress", Position: 1, WIPLimit: intPtr(3), Status: "in_progress"},
				{Name: "Done", Position: 2, Status: "done"},
			},
			DefaultConfig: map[string]any{}, ConfigSchema: map[string]any{},
		}, nil
	case "scrum":
		return &domain.BoardTypeDef{
			Type: "scrum", DisplayName: "Scrum Board", Icon: "🏃",
			DefaultColumns: []domain.BoardTypeColumn{
				{Name: "Backlog", Position: 0, Status: "open"},
				{Name: "Sprint", Position: 1, Status: "open"},
				{Name: "In Progress", Position: 2, Status: "in_progress"},
				{Name: "Review", Position: 3, Status: "in_progress"},
				{Name: "Done", Position: 4, Status: "done"},
			},
			DefaultConfig: map[string]any{"sprint_length_days": float64(14)},
			ConfigSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"sprint_length_days": map[string]any{"type": "integer", "minimum": float64(1), "maximum": float64(90)}},
				"additionalProperties": false,
			},
		}, nil
	case "calendar":
		return &domain.BoardTypeDef{
			Type: "calendar", DisplayName: "Calendar", Icon: "📅",
			DefaultColumns: nil,
			DefaultConfig:  map[string]any{"week_start": "monday"},
			ConfigSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"week_start": map[string]any{"type": "string", "enum": []any{"monday", "sunday"}}},
				"additionalProperties": false,
			},
		}, nil
	default:
		return nil, domain.ErrInvalidBoardType
	}
}

func newSvc(t *testing.T) (domain.ProjectService, *fakeRepo, *fakeCache) {
	t.Helper()
	repo := newFakeRepo()
	cache := newFakeCache()
	svc := domain.NewProjectService(repo, cache, fakeBoardTypeRegistry{})
	return svc, repo, cache
}

// ── Tests: Projects ───────────────────────────────────────────────────────────

func TestCreateProject_HappyPath(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "owner@example.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "My Project", "A description")
	require.NoError(t, err)
	assert.Equal(t, "My Project", proj.Name)
	assert.Equal(t, ownerID, proj.OwnerID)

	// Owner should be a member
	m, err := repo.GetMember(context.Background(), proj.ID, ownerID)
	require.NoError(t, err)
	assert.Equal(t, domain.RoleOwner, m.Role)
}

func TestCreateProject_ValidationFailed(t *testing.T) {
	svc, _, _ := newSvc(t)
	_, err := svc.CreateProject(context.Background(), uuid.New(), "", "desc")
	require.Error(t, err)
	var de *domain.Error
	require.True(t, errors.As(err, &de))
	assert.Equal(t, "validation_failed", de.Code)
}

func TestCreateProject_NameTooLong(t *testing.T) {
	svc, _, _ := newSvc(t)
	longName := make([]byte, 201)
	for i := range longName {
		longName[i] = 'a'
	}
	_, err := svc.CreateProject(context.Background(), uuid.New(), string(longName), "")
	require.Error(t, err)
}

func TestGetProject_PermissionDenied(t *testing.T) {
	svc, _, _ := newSvc(t)
	_, err := svc.GetProject(context.Background(), uuid.New(), uuid.New())
	require.Error(t, err)
	var de *domain.Error
	require.True(t, errors.As(err, &de))
	assert.Equal(t, "permission_denied", de.Code)
}

func TestGetProject_HappyPath(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "Test", "")
	require.NoError(t, err)

	got, err := svc.GetProject(context.Background(), proj.ID, ownerID)
	require.NoError(t, err)
	assert.Equal(t, proj.ID, got.ID)
}

func TestUpdateProject_HappyPath(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "Old Name", "")
	require.NoError(t, err)

	newName := "New Name"
	updated, err := svc.UpdateProject(context.Background(), proj.ID, ownerID, domain.ProjectPatch{Name: &newName})
	require.NoError(t, err)
	assert.Equal(t, "New Name", updated.Name)
}

func TestDeleteProject_HappyPath(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "ToDelete", "")
	require.NoError(t, err)

	require.NoError(t, svc.DeleteProject(context.Background(), proj.ID, ownerID))

	_, err = repo.GetProject(context.Background(), proj.ID)
	assert.ErrorIs(t, err, domain.ErrProjectNotFound)
}

func TestDeleteProject_ViewerPermissionDenied(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	viewerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")
	seedKnownUser(t, repo, viewerID, "v@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "Proj", "")
	require.NoError(t, err)

	_, err = svc.AddMember(context.Background(), proj.ID, ownerID, viewerID, "", domain.RoleViewer)
	require.NoError(t, err)

	err = svc.DeleteProject(context.Background(), proj.ID, viewerID)
	require.Error(t, err)
	var de *domain.Error
	require.True(t, errors.As(err, &de))
	assert.Equal(t, "permission_denied", de.Code)
}

// ── Tests: Members ────────────────────────────────────────────────────────────

func TestAddMember_HappyPath(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")
	seedKnownUser(t, repo, editorID, "e@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "Proj", "")
	require.NoError(t, err)

	m, err := svc.AddMember(context.Background(), proj.ID, ownerID, editorID, "", domain.RoleEditor)
	require.NoError(t, err)
	assert.Equal(t, domain.RoleEditor, m.Role)
}

func TestAddMember_ByEmail(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")
	seedKnownUser(t, repo, editorID, "editor@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "Proj", "")
	require.NoError(t, err)

	m, err := svc.AddMember(context.Background(), proj.ID, ownerID, uuid.Nil, "editor@x.com", domain.RoleEditor)
	require.NoError(t, err)
	assert.Equal(t, editorID, m.UserID)
}

func TestAddMember_AlreadyMember(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "Proj", "")
	require.NoError(t, err)

	// Owner is already a member
	_, err = svc.AddMember(context.Background(), proj.ID, ownerID, ownerID, "", domain.RoleEditor)
	require.Error(t, err)
	var de *domain.Error
	require.True(t, errors.As(err, &de))
	assert.Equal(t, "already_member", de.Code)
}

func TestAddMember_InvalidRole(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	targetID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")
	seedKnownUser(t, repo, targetID, "t@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "Proj", "")
	require.NoError(t, err)

	_, err = svc.AddMember(context.Background(), proj.ID, ownerID, targetID, "", "superadmin")
	require.Error(t, err)
	var de *domain.Error
	require.True(t, errors.As(err, &de))
	assert.Equal(t, "invalid_role", de.Code)
}

func TestRemoveMember_LastOwnerProtection(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "Proj", "")
	require.NoError(t, err)

	err = svc.RemoveMember(context.Background(), proj.ID, ownerID, ownerID)
	require.Error(t, err)
	var de *domain.Error
	require.True(t, errors.As(err, &de))
	assert.Equal(t, "last_owner_protected", de.Code)
}

func TestRemoveMember_SelfLeave(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")
	seedKnownUser(t, repo, editorID, "e@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "Proj", "")
	require.NoError(t, err)

	_, err = svc.AddMember(context.Background(), proj.ID, ownerID, editorID, "", domain.RoleEditor)
	require.NoError(t, err)

	// Editor leaves by themselves
	require.NoError(t, svc.RemoveMember(context.Background(), proj.ID, editorID, editorID))

	_, err = repo.GetMember(context.Background(), proj.ID, editorID)
	assert.ErrorIs(t, err, domain.ErrMemberNotFound)
}

func TestUpdateMemberRole_LastOwnerProtection(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "Proj", "")
	require.NoError(t, err)

	_, err = svc.UpdateMemberRole(context.Background(), proj.ID, ownerID, ownerID, domain.RoleEditor)
	require.Error(t, err)
	var de *domain.Error
	require.True(t, errors.As(err, &de))
	assert.Equal(t, "last_owner_protected", de.Code)
}

// ── Tests: Boards ─────────────────────────────────────────────────────────────

func TestCreateBoard_HappyPath_DefaultColumns(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "Proj", "")
	require.NoError(t, err)

	board, err := svc.CreateBoard(context.Background(), proj.ID, ownerID, domain.BoardInput{
		Name: "Kanban Board",
		Type: domain.BoardTypeKanban,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.BoardTypeKanban, board.Type)
	assert.Equal(t, "Kanban Board", board.Name)
	assert.Len(t, board.Columns, 3, "kanban strategy should inject 3 default columns")
}

func TestCreateBoard_InvalidType(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "Proj", "")
	require.NoError(t, err)

	_, err = svc.CreateBoard(context.Background(), proj.ID, ownerID, domain.BoardInput{
		Name: "Bad Board",
		Type: "flowboard",
	})
	require.Error(t, err)
	var de *domain.Error
	require.True(t, errors.As(err, &de))
	assert.Equal(t, "invalid_board_type", de.Code)
}

func TestCreateBoard_ScrumDefaultColumns(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	board, err := svc.CreateBoard(context.Background(), proj.ID, ownerID, domain.BoardInput{
		Name: "Sprint Board",
		Type: domain.BoardTypeScrum,
	})
	require.NoError(t, err)
	assert.Len(t, board.Columns, 5)
}

type unavailableRegistry struct{}

func (unavailableRegistry) GetType(context.Context, string) (*domain.BoardTypeDef, error) {
	return nil, domain.ErrBoardTypeRegistryUnavailable
}

func TestCreateBoard_RegistryUnavailable(t *testing.T) {
	repo := newFakeRepo()
	cache := newFakeCache()
	svc := domain.NewProjectService(repo, cache, unavailableRegistry{})
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	_, err = svc.CreateBoard(context.Background(), proj.ID, ownerID, domain.BoardInput{Name: "B", Type: "kanban"})
	require.Error(t, err)
	var de *domain.Error
	require.True(t, errors.As(err, &de))
	assert.Equal(t, "board_type_registry_unavailable", de.Code)
}

func TestDeleteBoard_HappyPath(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	board, err := svc.CreateBoard(context.Background(), proj.ID, ownerID, domain.BoardInput{Name: "B", Type: domain.BoardTypeKanban})
	require.NoError(t, err)

	require.NoError(t, svc.DeleteBoard(context.Background(), board.ID, ownerID))

	_, err = repo.GetBoard(context.Background(), board.ID)
	assert.ErrorIs(t, err, domain.ErrBoardNotFound)
}

// ── Tests: Permissions ────────────────────────────────────────────────────────

func TestGetPermissions_NonMember(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	ps, err := svc.GetPermissions(context.Background(), proj.ID, uuid.New())
	require.NoError(t, err)
	assert.False(t, ps.IsMember)
	assert.True(t, ps.ProjectExists)
	assert.Empty(t, ps.Permissions)
}

func TestGetPermissions_Owner(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	ps, err := svc.GetPermissions(context.Background(), proj.ID, ownerID)
	require.NoError(t, err)
	assert.True(t, ps.IsMember)
	assert.Equal(t, domain.RoleOwner, ps.Role)
	assert.Contains(t, ps.Permissions, "project:delete")
	assert.Contains(t, ps.Permissions, "member:invite")
}

func TestGetPermissions_CachesResult(t *testing.T) {
	svc, repo, fc := newSvc(t)
	ownerID := uuid.New()
	seedKnownUser(t, repo, ownerID, "o@x.com")

	proj, err := svc.CreateProject(context.Background(), ownerID, "P", "")
	require.NoError(t, err)

	// First call populates cache
	ps1, err := svc.GetPermissions(context.Background(), proj.ID, ownerID)
	require.NoError(t, err)

	// Verify cache was populated
	cached, err := fc.Get(context.Background(), proj.ID, ownerID)
	require.NoError(t, err)
	require.NotNil(t, cached)
	assert.Equal(t, ps1.Role, cached.Role)
}
