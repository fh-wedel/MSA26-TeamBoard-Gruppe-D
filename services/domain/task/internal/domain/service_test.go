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
	"github.com/teamboard/services/domain/task/internal/domain"
)

// ── Fake repository ───────────────────────────────────────────────────────────

type fakeRepo struct {
	mu           sync.Mutex
	tasks        map[uuid.UUID]*domain.Task
	comments     map[uuid.UUID]*domain.Comment
	attachments  map[uuid.UUID]*domain.Attachment
	history      []*domain.HistoryEntry
	outbox       []*domain.OutboxEvent
	knownBoards  map[uuid.UUID]*domain.KnownBoard
	knownColumns map[uuid.UUID]*domain.KnownColumn
	knownUsers   map[uuid.UUID]string // id → email
	deletedUsers map[uuid.UUID]bool
	processed    map[string]bool
	mentions     map[uuid.UUID][]uuid.UUID // commentID → []userID
	commentHist  []commentHistEntry
}

type commentHistEntry struct {
	histID    uuid.UUID
	commentID uuid.UUID
	body      string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		tasks:        make(map[uuid.UUID]*domain.Task),
		comments:     make(map[uuid.UUID]*domain.Comment),
		attachments:  make(map[uuid.UUID]*domain.Attachment),
		knownBoards:  make(map[uuid.UUID]*domain.KnownBoard),
		knownColumns: make(map[uuid.UUID]*domain.KnownColumn),
		knownUsers:   make(map[uuid.UUID]string),
		deletedUsers: make(map[uuid.UUID]bool),
		processed:    make(map[string]bool),
		mentions:     make(map[uuid.UUID][]uuid.UUID),
	}
}

func (r *fakeRepo) WithTransaction(ctx context.Context, fn func(context.Context, domain.Repository) error) error {
	return fn(ctx, r)
}

func (r *fakeRepo) CreateTask(_ context.Context, id, boardID, projectID uuid.UUID, columnID *uuid.UUID, title, description string, status domain.Status, priority domain.Priority, assigneeID *uuid.UUID, dueDate, startDate *time.Time, labels []string, position string, createdBy uuid.UUID) (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := &domain.Task{
		ID: id, BoardID: boardID, ProjectID: projectID, ColumnID: columnID,
		Title: title, Description: description, Status: status, Priority: priority,
		AssigneeID: assigneeID, DueDate: dueDate, StartDate: startDate, Labels: labels,
		Position: position, CreatedBy: createdBy,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	r.tasks[id] = t
	return t, nil
}

func (r *fakeRepo) GetTask(_ context.Context, id uuid.UUID) (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok || t.DeletedAt != nil {
		return nil, errors.New("not found")
	}
	return t, nil
}

func (r *fakeRepo) GetTaskWithCounts(_ context.Context, id uuid.UUID) (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok || t.DeletedAt != nil {
		return nil, errors.New("not found")
	}
	// Count comments and attachments.
	cc := 0
	for _, c := range r.comments {
		if c.TaskID == id && c.DeletedAt == nil {
			cc++
		}
	}
	ac := 0
	for _, a := range r.attachments {
		if a.TaskID == id {
			ac++
		}
	}
	copy := *t
	copy.CommentCount = cc
	copy.AttachmentCount = ac
	return &copy, nil
}

func (r *fakeRepo) ListTasksByBoard(_ context.Context, boardID uuid.UUID, _ domain.TaskFilter, _ *string, limit int) ([]*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Task
	for _, t := range r.tasks {
		if t.BoardID == boardID && t.DeletedAt == nil {
			out = append(out, t)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *fakeRepo) GetLastPositionInColumn(_ context.Context, _ uuid.UUID, _ *uuid.UUID) (string, error) {
	return "", nil
}

func (r *fakeRepo) GetPositionForRefs(_ context.Context, beforeID, afterID *uuid.UUID) (string, string, error) {
	return "", "", nil
}

func (r *fakeRepo) UpdateTask(_ context.Context, id uuid.UUID, patch domain.TaskPatch) (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok {
		return nil, errors.New("not found")
	}
	if patch.Title != nil {
		t.Title = *patch.Title
	}
	if patch.Description != nil {
		t.Description = *patch.Description
	}
	if patch.Priority != nil {
		t.Priority = *patch.Priority
	}
	if patch.DueDateSet {
		t.DueDate = patch.DueDate
	}
	if patch.Labels != nil {
		t.Labels = *patch.Labels
	}
	t.UpdatedAt = time.Now()
	return t, nil
}

func (r *fakeRepo) MoveTask(_ context.Context, id, columnID uuid.UUID, position string, status domain.Status) (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok {
		return nil, errors.New("not found")
	}
	t.ColumnID = &columnID
	t.Position = position
	t.Status = status
	t.UpdatedAt = time.Now()
	return t, nil
}

func (r *fakeRepo) AssignTask(_ context.Context, id uuid.UUID, assigneeID *uuid.UUID) (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok {
		return nil, errors.New("not found")
	}
	t.AssigneeID = assigneeID
	t.UpdatedAt = time.Now()
	return t, nil
}

func (r *fakeRepo) SoftDeleteTask(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok {
		return errors.New("not found")
	}
	now := time.Now()
	t.DeletedAt = &now
	return nil
}

func (r *fakeRepo) SoftDeleteTasksByProject(_ context.Context, projectID uuid.UUID) ([]uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []uuid.UUID
	for _, t := range r.tasks {
		if t.ProjectID == projectID && t.DeletedAt == nil {
			now := time.Now()
			t.DeletedAt = &now
			ids = append(ids, t.ID)
		}
	}
	return ids, nil
}

func (r *fakeRepo) SoftDeleteTasksByBoard(_ context.Context, boardID uuid.UUID) ([]uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []uuid.UUID
	for _, t := range r.tasks {
		if t.BoardID == boardID && t.DeletedAt == nil {
			now := time.Now()
			t.DeletedAt = &now
			ids = append(ids, t.ID)
		}
	}
	return ids, nil
}

func (r *fakeRepo) ClearAssigneeForUser(_ context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []uuid.UUID
	for _, t := range r.tasks {
		if t.AssigneeID != nil && *t.AssigneeID == userID {
			t.AssigneeID = nil
			ids = append(ids, t.ID)
		}
	}
	return ids, nil
}

func (r *fakeRepo) NullifyColumnReferences(_ context.Context, columnID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tasks {
		if t.ColumnID != nil && *t.ColumnID == columnID {
			t.ColumnID = nil
		}
	}
	return nil
}

// Comments

func (r *fakeRepo) CreateComment(_ context.Context, id, taskID, authorID uuid.UUID, body string) (*domain.Comment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := &domain.Comment{ID: id, TaskID: taskID, AuthorID: authorID, Body: body, CreatedAt: time.Now()}
	r.comments[id] = c
	return c, nil
}

func (r *fakeRepo) GetComment(_ context.Context, id uuid.UUID) (*domain.Comment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.comments[id]
	if !ok || c.DeletedAt != nil {
		return nil, errors.New("not found")
	}
	return c, nil
}

func (r *fakeRepo) ListCommentsByTask(_ context.Context, taskID uuid.UUID) ([]*domain.Comment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Comment
	for _, c := range r.comments {
		if c.TaskID == taskID && c.DeletedAt == nil {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *fakeRepo) UpdateComment(_ context.Context, id uuid.UUID, body string) (*domain.Comment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.comments[id]
	if !ok {
		return nil, errors.New("not found")
	}
	now := time.Now()
	c.Body = body
	c.EditedAt = &now
	return c, nil
}

func (r *fakeRepo) SoftDeleteComment(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.comments[id]
	if !ok {
		return errors.New("not found")
	}
	now := time.Now()
	c.DeletedAt = &now
	return nil
}

func (r *fakeRepo) ArchiveCommentBody(_ context.Context, histID, commentID uuid.UUID, body string) error {
	r.commentHist = append(r.commentHist, commentHistEntry{histID: histID, commentID: commentID, body: body})
	return nil
}

func (r *fakeRepo) AddMention(_ context.Context, commentID, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mentions[commentID] = append(r.mentions[commentID], userID)
	return nil
}

func (r *fakeRepo) ListMentionsByComment(_ context.Context, commentID uuid.UUID) ([]uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mentions[commentID], nil
}

func (r *fakeRepo) ClearMentions(_ context.Context, commentID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.mentions, commentID)
	return nil
}

// Attachments

func (r *fakeRepo) CreateAttachment(_ context.Context, id, taskID, documentID, addedBy uuid.UUID) (*domain.Attachment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := &domain.Attachment{ID: id, TaskID: taskID, DocumentID: documentID, AddedBy: addedBy, AddedAt: time.Now()}
	r.attachments[id] = a
	return a, nil
}

func (r *fakeRepo) GetAttachment(_ context.Context, id uuid.UUID) (*domain.Attachment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.attachments[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return a, nil
}

func (r *fakeRepo) ListAttachmentsByTask(_ context.Context, taskID uuid.UUID) ([]*domain.Attachment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Attachment
	for _, a := range r.attachments {
		if a.TaskID == taskID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (r *fakeRepo) DeleteAttachment(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.attachments, id)
	return nil
}

func (r *fakeRepo) DeleteAttachmentsByDocument(_ context.Context, documentID uuid.UUID) ([]struct{ ID, TaskID uuid.UUID }, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var pairs []struct{ ID, TaskID uuid.UUID }
	for id, a := range r.attachments {
		if a.DocumentID == documentID {
			pairs = append(pairs, struct{ ID, TaskID uuid.UUID }{id, a.TaskID})
			delete(r.attachments, id)
		}
	}
	return pairs, nil
}

// History

func (r *fakeRepo) CreateHistoryEntry(_ context.Context, id, taskID, actorID uuid.UUID, changeType domain.ChangeType, diff map[string]any) error {
	r.history = append(r.history, &domain.HistoryEntry{
		ID: id, TaskID: taskID, ActorID: actorID, ChangeType: changeType, Diff: diff, OccurredAt: time.Now(),
	})
	return nil
}

func (r *fakeRepo) ListTaskHistory(_ context.Context, taskID uuid.UUID, limit int) ([]*domain.HistoryEntry, error) {
	var out []*domain.HistoryEntry
	for _, e := range r.history {
		if e.TaskID == taskID {
			out = append(out, e)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// Known boards / columns / users

func (r *fakeRepo) UpsertKnownBoard(_ context.Context, id, projectID uuid.UUID, name, bType string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.knownBoards[id] = &domain.KnownBoard{ID: id, ProjectID: projectID, Name: name, Type: bType}
	return nil
}

func (r *fakeRepo) GetKnownBoard(_ context.Context, id uuid.UUID) (*domain.KnownBoard, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.knownBoards[id]
	if !ok || b.DeletedAt != nil {
		return nil, errors.New("not found")
	}
	return b, nil
}

func (r *fakeRepo) MarkBoardDeleted(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.knownBoards[id]
	if !ok {
		return nil
	}
	now := time.Now()
	b.DeletedAt = &now
	return nil
}

func (r *fakeRepo) UpsertKnownColumn(_ context.Context, id, boardID uuid.UUID, name string, position int, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.knownColumns[id] = &domain.KnownColumn{ID: id, BoardID: boardID, Name: name, Position: position, Status: status}
	return nil
}

func (r *fakeRepo) GetKnownColumn(_ context.Context, id uuid.UUID) (*domain.KnownColumn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.knownColumns[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return c, nil
}

func (r *fakeRepo) ColumnBelongsToBoard(_ context.Context, columnID, boardID uuid.UUID) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.knownColumns[columnID]
	if !ok {
		return false, nil
	}
	return c.BoardID == boardID, nil
}

func (r *fakeRepo) DeleteKnownColumn(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.knownColumns, id)
	return nil
}

func (r *fakeRepo) UpsertKnownUser(_ context.Context, id uuid.UUID, email string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.knownUsers[id] = email
	delete(r.deletedUsers, id)
	return nil
}

func (r *fakeRepo) KnownUserExists(_ context.Context, id uuid.UUID) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.knownUsers[id]
	if !ok {
		return false, nil
	}
	return !r.deletedUsers[id], nil
}

func (r *fakeRepo) MarkKnownUserDeleted(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deletedUsers[id] = true
	return nil
}

// Outbox

func (r *fakeRepo) InsertOutboxEvent(_ context.Context, id, aggregateID uuid.UUID, eventType string, payload []byte) error {
	r.outbox = append(r.outbox, &domain.OutboxEvent{ID: id, AggregateID: aggregateID, EventType: eventType, Payload: payload})
	return nil
}

func (r *fakeRepo) GetUnpublishedEvents(_ context.Context, limit int32) ([]*domain.OutboxEvent, error) {
	return r.outbox, nil
}

func (r *fakeRepo) MarkEventPublished(_ context.Context, id uuid.UUID) error {
	return nil
}

// Idempotency

func (r *fakeRepo) WasEventProcessed(_ context.Context, eventID string) (bool, error) {
	return r.processed[eventID], nil
}

func (r *fakeRepo) MarkEventProcessed(_ context.Context, eventID string) error {
	r.processed[eventID] = true
	return nil
}

// ── Fake project client ───────────────────────────────────────────────────────

type fakeProjClient struct {
	perms map[string]*domain.PermissionSet // key: "projectID:userID"
}

func newFakeProjClient() *fakeProjClient {
	return &fakeProjClient{perms: make(map[string]*domain.PermissionSet)}
}

func (c *fakeProjClient) allow(projectID, userID uuid.UUID, permissions ...string) {
	c.perms[projectID.String()+":"+userID.String()] = &domain.PermissionSet{
		IsMember:      true,
		Permissions:   permissions,
		ProjectExists: true,
	}
}

func (c *fakeProjClient) GetPermissions(_ context.Context, projectID, userID uuid.UUID) (*domain.PermissionSet, error) {
	key := projectID.String() + ":" + userID.String()
	if ps, ok := c.perms[key]; ok {
		return ps, nil
	}
	return &domain.PermissionSet{IsMember: false, ProjectExists: true}, nil
}

// ── Fake document client ──────────────────────────────────────────────────────

type fakeDocClient struct {
	docs map[uuid.UUID]*domain.DocumentInfo
}

func newFakeDocClient() *fakeDocClient {
	return &fakeDocClient{docs: make(map[uuid.UUID]*domain.DocumentInfo)}
}

func (c *fakeDocClient) GetDocumentInfo(_ context.Context, id uuid.UUID) (*domain.DocumentInfo, error) {
	d, ok := c.docs[id]
	if !ok {
		return nil, domain.ErrDocumentUnreachable
	}
	return d, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func makeService(t *testing.T) (*domain.TaskService, *fakeRepo, *fakeProjClient, *fakeDocClient) {
	t.Helper()
	repo := newFakeRepo()
	proj := newFakeProjClient()
	doc := newFakeDocClient()
	svc := domain.NewTaskService(repo, proj, doc)
	return &svc, repo, proj, doc
}

// seedBoard adds a known board and column to the fake repo.
func seedBoard(repo *fakeRepo, boardID, projectID uuid.UUID) {
	repo.knownBoards[boardID] = &domain.KnownBoard{ID: boardID, ProjectID: projectID, Name: "Test Board", Type: "kanban"}
}

func seedColumn(repo *fakeRepo, colID, boardID uuid.UUID, name string) {
	repo.knownColumns[colID] = &domain.KnownColumn{ID: colID, BoardID: boardID, Name: name, Position: 1}
}

func seedColumnWithStatus(repo *fakeRepo, colID, boardID uuid.UUID, name, status string) {
	repo.knownColumns[colID] = &domain.KnownColumn{ID: colID, BoardID: boardID, Name: name, Position: 1, Status: status}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestTaskService_CreateTask_ExplicitColumnStatusWins(t *testing.T) {
	svc, repo, proj, _ := makeService(t)

	boardID := uuid.New()
	projectID := uuid.New()
	userID := uuid.New()
	colID := uuid.New()

	seedBoard(repo, boardID, projectID)
	// Column name "Shipped" would derive to open, but the explicit status is done.
	seedColumnWithStatus(repo, colID, boardID, "Shipped", "done")
	proj.allow(projectID, userID, "task:create", "task:read")

	task, err := (*svc).CreateTask(context.Background(), userID, domain.CreateTaskInput{
		BoardID:  boardID,
		ColumnID: &colID,
		Title:    "Done task",
		Priority: domain.PriorityMedium,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDone, task.Status)
}

func TestTaskService_CreateTask_HappyPath(t *testing.T) {
	svc, repo, proj, _ := makeService(t)

	boardID := uuid.New()
	projectID := uuid.New()
	userID := uuid.New()
	colID := uuid.New()

	seedBoard(repo, boardID, projectID)
	seedColumn(repo, colID, boardID, "In Progress")
	proj.allow(projectID, userID, "task:create", "task:read")

	task, err := (*svc).CreateTask(context.Background(), userID, domain.CreateTaskInput{
		BoardID:     boardID,
		ColumnID:    &colID,
		Title:       "My task",
		Description: "details",
		Priority:    domain.PriorityHigh,
		Labels:      []string{"bug"},
	})

	require.NoError(t, err)
	assert.Equal(t, "My task", task.Title)
	assert.Equal(t, domain.PriorityHigh, task.Priority)
	assert.Equal(t, domain.StatusInProgress, task.Status)
	assert.Equal(t, []string{"bug"}, task.Labels)
	assert.Equal(t, boardID, task.BoardID)
	assert.Equal(t, projectID, task.ProjectID)

	// Outbox event emitted.
	assert.Len(t, repo.outbox, 1)
	assert.Equal(t, "task.created", repo.outbox[0].EventType)
}

func TestTaskService_CreateTask_DefaultPriority(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	boardID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	seedBoard(repo, boardID, projectID)
	proj.allow(projectID, userID, "task:create")

	task, err := (*svc).CreateTask(context.Background(), userID, domain.CreateTaskInput{
		BoardID: boardID,
		Title:   "No priority set",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.PriorityMedium, task.Priority)
}

func TestTaskService_CreateTask_EmptyTitle_Fails(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	boardID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	seedBoard(repo, boardID, projectID)
	proj.allow(projectID, userID, "task:create")

	_, err := (*svc).CreateTask(context.Background(), userID, domain.CreateTaskInput{
		BoardID: boardID,
		Title:   "",
	})
	require.Error(t, err)
	assertDomainError(t, err, "validation_failed")
}

func TestTaskService_CreateTask_UnknownBoard_Fails(t *testing.T) {
	svc, _, _, _ := makeService(t)
	_, err := (*svc).CreateTask(context.Background(), uuid.New(), domain.CreateTaskInput{
		BoardID: uuid.New(),
		Title:   "task",
	})
	assertDomainError(t, err, "board_unknown")
}

func TestTaskService_CreateTask_ColumnNotInBoard_Fails(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	boardID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	seedBoard(repo, boardID, projectID)
	proj.allow(projectID, userID, "task:create")
	otherBoardCol := uuid.New()
	seedColumn(repo, otherBoardCol, uuid.New(), "Other")

	_, err := (*svc).CreateTask(context.Background(), userID, domain.CreateTaskInput{
		BoardID:  boardID,
		ColumnID: &otherBoardCol,
		Title:    "task",
	})
	assertDomainError(t, err, "column_not_in_board")
}

func TestTaskService_CreateTask_PermissionDenied(t *testing.T) {
	svc, repo, _, _ := makeService(t)
	boardID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	seedBoard(repo, boardID, projectID)

	_, err := (*svc).CreateTask(context.Background(), userID, domain.CreateTaskInput{
		BoardID: boardID,
		Title:   "task",
	})
	assertDomainError(t, err, "permission_denied")
}

func TestTaskService_GetTask_HappyPath(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)

	got, err := (*svc).GetTask(context.Background(), task.ID, task.CreatedBy)
	require.NoError(t, err)
	assert.Equal(t, task.ID, got.ID)
}

func TestTaskService_GetTask_NotFound(t *testing.T) {
	svc, _, _, _ := makeService(t)
	_, err := (*svc).GetTask(context.Background(), uuid.New(), uuid.New())
	assertDomainError(t, err, "task_not_found")
}

func TestTaskService_UpdateTask_HappyPath(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)

	newTitle := "Updated"
	newPriority := domain.PriorityLow
	updated, err := (*svc).UpdateTask(context.Background(), task.ID, task.CreatedBy, domain.TaskPatch{
		Title:    &newTitle,
		Priority: &newPriority,
	})

	require.NoError(t, err)
	assert.Equal(t, "Updated", updated.Title)
	assert.Equal(t, domain.PriorityLow, updated.Priority)
}

func TestTaskService_UpdateTask_InvalidPriority(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)

	bad := domain.Priority("extreme")
	_, err := (*svc).UpdateTask(context.Background(), task.ID, task.CreatedBy, domain.TaskPatch{Priority: &bad})
	assertDomainError(t, err, "validation_failed")
}

func TestTaskService_DeleteTask_HappyPath(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "task:delete")

	err := (*svc).DeleteTask(context.Background(), task.ID, task.CreatedBy)
	require.NoError(t, err)

	_, err = repo.GetTask(context.Background(), task.ID)
	assert.Error(t, err, "task should be soft-deleted")
}

func TestTaskService_MoveTask_HappyPath(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	newColID := uuid.New()
	seedColumn(repo, newColID, task.BoardID, "Done")
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "task:update")

	moved, err := (*svc).MoveTask(context.Background(), task.ID, task.CreatedBy, newColID, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, moved.ColumnID)
	assert.Equal(t, newColID, *moved.ColumnID)
	assert.Equal(t, domain.StatusDone, moved.Status)
}

func TestTaskService_MoveTask_EmitsStatusChangedEvent(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj) // initial status is "open" (no column)
	newColID := uuid.New()
	seedColumn(repo, newColID, task.BoardID, "In Progress")
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "task:update")

	_, err := (*svc).MoveTask(context.Background(), task.ID, task.CreatedBy, newColID, nil, nil)
	require.NoError(t, err)

	eventTypes := collectEventTypes(repo)
	assert.Contains(t, eventTypes, "task.moved")
	assert.Contains(t, eventTypes, "task.status.changed")
}

func TestTaskService_AssignTask_HappyPath(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	assignee := uuid.New()
	proj.allow(task.ProjectID, assignee) // member but no specific perms needed for being assignee
	repo.knownUsers[assignee] = "assignee@test.com"
	// Make assignee a member
	proj.perms[task.ProjectID.String()+":"+assignee.String()] = &domain.PermissionSet{
		IsMember: true, ProjectExists: true,
	}
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "task:update")

	updated, err := (*svc).AssignTask(context.Background(), task.ID, task.CreatedBy, &assignee)
	require.NoError(t, err)
	require.NotNil(t, updated.AssigneeID)
	assert.Equal(t, assignee, *updated.AssigneeID)
}

func TestTaskService_AssignTask_NonMember_Fails(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "task:update")
	nonMember := uuid.New()

	_, err := (*svc).AssignTask(context.Background(), task.ID, task.CreatedBy, &nonMember)
	assertDomainError(t, err, "assignee_not_member")
}

func TestTaskService_AssignTask_Unassign(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "task:update")

	updated, err := (*svc).AssignTask(context.Background(), task.ID, task.CreatedBy, nil)
	require.NoError(t, err)
	assert.Nil(t, updated.AssigneeID)

	eventTypes := collectEventTypes(repo)
	assert.Contains(t, eventTypes, "task.unassigned")
}

func TestTaskService_CreateComment_HappyPath(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "comment:create")

	comment, err := (*svc).CreateComment(context.Background(), task.ID, task.CreatedBy, "Hello world")
	require.NoError(t, err)
	assert.Equal(t, "Hello world", comment.Body)
	assert.Equal(t, task.ID, comment.TaskID)

	eventTypes := collectEventTypes(repo)
	assert.Contains(t, eventTypes, "task.commented")
}

func TestTaskService_CreateComment_EmptyBody_Fails(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "comment:create")

	_, err := (*svc).CreateComment(context.Background(), task.ID, task.CreatedBy, "")
	assertDomainError(t, err, "validation_failed")
}

func TestTaskService_UpdateComment_HappyPath(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "comment:create", "comment:read")

	comment, err := (*svc).CreateComment(context.Background(), task.ID, task.CreatedBy, "original")
	require.NoError(t, err)

	repo.outbox = nil // reset

	updated, err := (*svc).UpdateComment(context.Background(), comment.ID, task.CreatedBy, "edited")
	require.NoError(t, err)
	assert.Equal(t, "edited", updated.Body)

	// Original body archived.
	require.Len(t, repo.commentHist, 1)
	assert.Equal(t, "original", repo.commentHist[0].body)
}

func TestTaskService_UpdateComment_NotAuthor_Fails(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "comment:create", "comment:read")

	comment, err := (*svc).CreateComment(context.Background(), task.ID, task.CreatedBy, "original")
	require.NoError(t, err)

	otherUser := uuid.New()
	_, err = (*svc).UpdateComment(context.Background(), comment.ID, otherUser, "hacked")
	assertDomainError(t, err, "not_comment_author")
}

func TestTaskService_DeleteComment_HappyPath(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "comment:create")

	comment, err := (*svc).CreateComment(context.Background(), task.ID, task.CreatedBy, "to delete")
	require.NoError(t, err)
	repo.outbox = nil

	err = (*svc).DeleteComment(context.Background(), comment.ID, task.CreatedBy)
	require.NoError(t, err)

	_, err = repo.GetComment(context.Background(), comment.ID)
	assert.Error(t, err)
}

func TestTaskService_AddAttachment_HappyPath(t *testing.T) {
	svc, repo, proj, doc := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "task:update")
	docID := uuid.New()
	doc.docs[docID] = &domain.DocumentInfo{ID: docID, ProjectID: task.ProjectID}

	att, err := (*svc).AddAttachment(context.Background(), task.ID, task.CreatedBy, docID)
	require.NoError(t, err)
	assert.Equal(t, docID, att.DocumentID)
	assert.Equal(t, task.ID, att.TaskID)
}

func TestTaskService_AddAttachment_WrongProject_Fails(t *testing.T) {
	svc, repo, proj, doc := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "task:update")
	docID := uuid.New()
	doc.docs[docID] = &domain.DocumentInfo{ID: docID, ProjectID: uuid.New()} // different project

	_, err := (*svc).AddAttachment(context.Background(), task.ID, task.CreatedBy, docID)
	assertDomainError(t, err, "document_not_in_project")
}

func TestTaskService_RemoveAttachment_HappyPath(t *testing.T) {
	svc, repo, proj, doc := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "task:update")
	docID := uuid.New()
	doc.docs[docID] = &domain.DocumentInfo{ID: docID, ProjectID: task.ProjectID}

	att, err := (*svc).AddAttachment(context.Background(), task.ID, task.CreatedBy, docID)
	require.NoError(t, err)

	repo.outbox = nil
	err = (*svc).RemoveAttachment(context.Background(), att.ID, task.CreatedBy)
	require.NoError(t, err)

	_, err = repo.GetAttachment(context.Background(), att.ID)
	assert.Error(t, err)

	eventTypes := collectEventTypes(repo)
	assert.Contains(t, eventTypes, "task.attachment.removed")
}

func TestTaskService_GetTaskHistory_HappyPath(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create")

	entries, err := (*svc).GetTaskHistory(context.Background(), task.ID, task.CreatedBy)
	require.NoError(t, err)
	// CreateTask inserts one history entry.
	assert.Len(t, entries, 1)
	assert.Equal(t, domain.ChangeCreated, entries[0].ChangeType)
}

func TestTaskService_ListTasks_HappyPath(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create")

	tasks, cursor, err := (*svc).ListTasks(context.Background(), task.BoardID, task.CreatedBy,
		domain.TaskFilter{}, domain.Pagination{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Nil(t, cursor)
}

func TestTaskService_ListComments_HappyPath(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "comment:create", "comment:read")

	_, err := (*svc).CreateComment(context.Background(), task.ID, task.CreatedBy, "hello")
	require.NoError(t, err)

	comments, err := (*svc).ListComments(context.Background(), task.ID, task.CreatedBy)
	require.NoError(t, err)
	assert.Len(t, comments, 1)
}

func TestTaskService_ListAttachments_HappyPath(t *testing.T) {
	svc, repo, proj, doc := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "task:update")
	docID := uuid.New()
	doc.docs[docID] = &domain.DocumentInfo{ID: docID, ProjectID: task.ProjectID}

	_, err := (*svc).AddAttachment(context.Background(), task.ID, task.CreatedBy, docID)
	require.NoError(t, err)

	atts, err := (*svc).ListAttachments(context.Background(), task.ID, task.CreatedBy)
	require.NoError(t, err)
	assert.Len(t, atts, 1)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// seedTask creates a task via the service so that history and outbox entries
// are properly populated.
func seedTask(repo *fakeRepo, proj *fakeProjClient) *domain.Task {
	boardID := uuid.New()
	projectID := uuid.New()
	userID := uuid.New()

	seedBoard(repo, boardID, projectID)
	proj.allow(projectID, userID, "task:read", "task:create", "task:update", "task:delete")

	svc := domain.NewTaskService(repo, proj, newFakeDocClient())
	task, err := svc.CreateTask(context.Background(), userID, domain.CreateTaskInput{
		BoardID:  boardID,
		Title:    "Seeded task",
		Priority: domain.PriorityMedium,
		Labels:   []string{},
	})
	if err != nil {
		panic("seedTask failed: " + err.Error())
	}
	return task
}

func assertDomainError(t *testing.T, err error, code string) {
	t.Helper()
	require.Error(t, err)
	var de *domain.Error
	require.True(t, errors.As(err, &de), "expected domain.Error, got: %T %v", err, err)
	assert.Equal(t, code, de.Code)
}

func collectEventTypes(repo *fakeRepo) []string {
	types := make([]string, len(repo.outbox))
	for i, e := range repo.outbox {
		types[i] = e.EventType
	}
	return types
}
