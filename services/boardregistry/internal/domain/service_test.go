package domain

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepo is an in-memory Repository for unit tests.
type fakeRepo struct {
	types  map[string]*BoardTypeDef
	events []string
}

func newFakeRepo() *fakeRepo { return &fakeRepo{types: map[string]*BoardTypeDef{}} }

func (f *fakeRepo) CreateBoardType(_ context.Context, def *BoardTypeDef) (*BoardTypeDef, error) {
	cp := *def
	f.types[def.Type] = &cp
	return &cp, nil
}

func (f *fakeRepo) GetBoardType(_ context.Context, typ string) (*BoardTypeDef, error) {
	d, ok := f.types[typ]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *d
	return &cp, nil
}

func (f *fakeRepo) ListBoardTypes(_ context.Context) ([]*BoardTypeDef, error) {
	out := make([]*BoardTypeDef, 0, len(f.types))
	for _, d := range f.types {
		cp := *d
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeRepo) UpdateBoardType(_ context.Context, def *BoardTypeDef) (*BoardTypeDef, error) {
	if _, ok := f.types[def.Type]; !ok {
		return nil, ErrNotFound
	}
	cp := *def
	f.types[def.Type] = &cp
	return &cp, nil
}

func (f *fakeRepo) DeleteBoardType(_ context.Context, typ string) error {
	if _, ok := f.types[typ]; !ok {
		return ErrNotFound
	}
	delete(f.types, typ)
	return nil
}

func (f *fakeRepo) InsertOutboxEvent(_ context.Context, _, _ uuid.UUID, eventType string, _ []byte) error {
	f.events = append(f.events, eventType)
	return nil
}
func (f *fakeRepo) GetUnpublishedEvents(context.Context, int32) ([]*OutboxEvent, error) {
	return nil, nil
}
func (f *fakeRepo) MarkEventPublished(context.Context, uuid.UUID) error { return nil }

func (f *fakeRepo) WithTransaction(ctx context.Context, fn func(context.Context, Repository) error) error {
	return fn(ctx, f)
}

func validInput(typ string) RegisterInput {
	return RegisterInput{
		Type:        typ,
		DisplayName: "Test Board",
		Icon:        "🧪",
		DefaultColumns: []ColumnDef{
			{Name: "To Do", Position: 0, Status: "open"},
			{Name: "Done", Position: 1, Status: "done"},
		},
		DefaultConfig: map[string]any{},
		ConfigSchema:  map[string]any{},
	}
}

func TestRegister_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	def, err := svc.Register(context.Background(), validInput("gantt"))
	require.NoError(t, err)
	assert.Equal(t, "gantt", def.Type)
	assert.False(t, def.BuiltIn)
	assert.NotEqual(t, uuid.Nil, def.ID)
	assert.Contains(t, repo.events, "boardtype.registered")
}

func TestRegister_DuplicateRejected(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	_, err := svc.Register(context.Background(), validInput("gantt"))
	require.NoError(t, err)

	_, err = svc.Register(context.Background(), validInput("gantt"))
	assert.ErrorIs(t, err, ErrAlreadyExists)
}

func TestRegister_InvalidSlug(t *testing.T) {
	svc := NewService(newFakeRepo())
	in := validInput("Not A Slug")
	_, err := svc.Register(context.Background(), in)
	var de *Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "validation_failed", de.Code)
}

func TestRegister_InvalidColumnStatus(t *testing.T) {
	svc := NewService(newFakeRepo())
	in := validInput("gantt")
	in.DefaultColumns[0].Status = "shipped"
	_, err := svc.Register(context.Background(), in)
	var de *Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "validation_failed", de.Code)
}

func TestRegister_DefaultConfigViolatesSchema(t *testing.T) {
	svc := NewService(newFakeRepo())
	in := validInput("scrum2")
	in.ConfigSchema = map[string]any{
		"type":       "object",
		"properties": map[string]any{"sprint_length_days": map[string]any{"type": "integer", "maximum": float64(90)}},
	}
	in.DefaultConfig = map[string]any{"sprint_length_days": float64(999)}
	_, err := svc.Register(context.Background(), in)
	var de *Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "validation_failed", de.Code)
}

func TestUpdate_BuiltInImmutable(t *testing.T) {
	repo := newFakeRepo()
	repo.types["kanban"] = &BoardTypeDef{Type: "kanban", DisplayName: "Kanban", BuiltIn: true}
	svc := NewService(repo)

	name := "Hacked"
	_, err := svc.Update(context.Background(), "kanban", UpdatePatch{DisplayName: &name})
	assert.ErrorIs(t, err, ErrBuiltInImmutable)
}

func TestDelete_BuiltInImmutable(t *testing.T) {
	repo := newFakeRepo()
	repo.types["kanban"] = &BoardTypeDef{Type: "kanban", DisplayName: "Kanban", BuiltIn: true}
	svc := NewService(repo)

	err := svc.Delete(context.Background(), "kanban")
	assert.ErrorIs(t, err, ErrBuiltInImmutable)
}

func TestListAndGet(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	_, err := svc.Register(context.Background(), validInput("gantt"))
	require.NoError(t, err)
	_, err = svc.Register(context.Background(), validInput("timeline"))
	require.NoError(t, err)

	list, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 2)

	def, err := svc.Get(context.Background(), "gantt")
	require.NoError(t, err)
	assert.Equal(t, "gantt", def.Type)

	_, err = svc.Get(context.Background(), "missing")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRegister_InvalidDisplayName(t *testing.T) {
	svc := NewService(newFakeRepo())
	in := validInput("gantt")
	in.DisplayName = ""
	_, err := svc.Register(context.Background(), in)
	var de *Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "validation_failed", de.Code)
}

func TestRegister_DuplicateColumnPosition(t *testing.T) {
	svc := NewService(newFakeRepo())
	in := validInput("gantt")
	in.DefaultColumns[1].Position = 0 // collide with first column
	_, err := svc.Register(context.Background(), in)
	var de *Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "validation_failed", de.Code)
}

func TestRegister_NegativeWIPLimit(t *testing.T) {
	svc := NewService(newFakeRepo())
	in := validInput("gantt")
	neg := -1
	in.DefaultColumns[0].WIPLimit = &neg
	_, err := svc.Register(context.Background(), in)
	var de *Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "validation_failed", de.Code)
}

func TestRegister_InvalidConfigSchema(t *testing.T) {
	svc := NewService(newFakeRepo())
	in := validInput("gantt")
	// "type" must be a string/array; a number makes the schema itself invalid.
	in.ConfigSchema = map[string]any{"type": float64(123)}
	_, err := svc.Register(context.Background(), in)
	var de *Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "validation_failed", de.Code)
}

func TestUpdate_NotFound(t *testing.T) {
	svc := NewService(newFakeRepo())
	name := "x"
	_, err := svc.Update(context.Background(), "missing", UpdatePatch{DisplayName: &name})
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDelete_NotFound(t *testing.T) {
	svc := NewService(newFakeRepo())
	err := svc.Delete(context.Background(), "missing")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRegister_ValidPresentation(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	in := validInput("roadmap")
	in.Presentation = map[string]any{
		"view":        "calendar",
		"view_config": map[string]any{"date_field": "due_date", "week_start": "monday"},
		"card":        map[string]any{"fields": []any{"priority", "labels"}, "color_by": "priority"},
	}
	def, err := svc.Register(context.Background(), in)
	require.NoError(t, err)
	assert.Equal(t, "calendar", def.Presentation["view"])
}

func TestRegister_InvalidPresentation(t *testing.T) {
	cases := map[string]map[string]any{
		"unknown view":         {"view": "spreadsheet"},
		"bad view_config key":  {"view": "calendar", "view_config": map[string]any{"group_by": "column"}},
		"bad week_start":       {"view": "calendar", "view_config": map[string]any{"week_start": "tuesday"}},
		"bad card color_by":    {"card": map[string]any{"color_by": "rainbow"}},
		"unknown top-level key": {"layout": "grid"},
	}
	for name, pres := range cases {
		t.Run(name, func(t *testing.T) {
			svc := NewService(newFakeRepo())
			in := validInput("roadmap")
			in.Presentation = pres
			_, err := svc.Register(context.Background(), in)
			var de *Error
			require.ErrorAs(t, err, &de)
			assert.Equal(t, "validation_failed", de.Code)
		})
	}
}

func TestUpdate_PresentationValidated(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	_, err := svc.Register(context.Background(), validInput("roadmap"))
	require.NoError(t, err)

	bad := map[string]any{"view": "nope"}
	_, err = svc.Update(context.Background(), "roadmap", UpdatePatch{Presentation: &bad})
	var de *Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "validation_failed", de.Code)
}

func TestUpdateAndDelete_CustomType(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	_, err := svc.Register(context.Background(), validInput("gantt"))
	require.NoError(t, err)

	name := "Gantt Chart"
	updated, err := svc.Update(context.Background(), "gantt", UpdatePatch{DisplayName: &name})
	require.NoError(t, err)
	assert.Equal(t, "Gantt Chart", updated.DisplayName)
	assert.Contains(t, repo.events, "boardtype.updated")

	require.NoError(t, svc.Delete(context.Background(), "gantt"))
	assert.Contains(t, repo.events, "boardtype.deleted")
	_, err = svc.Get(context.Background(), "gantt")
	assert.ErrorIs(t, err, ErrNotFound)
}
