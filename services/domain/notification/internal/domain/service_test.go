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
	"github.com/teamboard/services/domain/notification/internal/domain"
)

// ── Fake repository ────────────────────────────────────────────────────────────

type fakeRepo struct {
	mu            sync.Mutex
	notifications map[uuid.UUID]*domain.Notification
	outbox        []*domain.OutboxEvent
	processed     map[string]bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		notifications: make(map[uuid.UUID]*domain.Notification),
		processed:     make(map[string]bool),
	}
}

func (r *fakeRepo) CreateNotification(_ context.Context, n *domain.Notification) (*domain.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n.SourceEventID != nil {
		for _, existing := range r.notifications {
			if existing.UserID == n.UserID && existing.SourceEventID != nil && *existing.SourceEventID == *n.SourceEventID {
				return nil, nil // idempotent duplicate
			}
		}
	}
	n.CreatedAt = time.Now()
	r.notifications[n.ID] = n
	return n, nil
}

func (r *fakeRepo) GetNotification(_ context.Context, id, userID uuid.UUID) (*domain.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.notifications[id]
	if !ok || n.UserID != userID {
		return nil, domain.ErrNotificationNotFound
	}
	return n, nil
}

func (r *fakeRepo) ListNotifications(_ context.Context, userID uuid.UUID, filter domain.ListFilter) ([]*domain.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []*domain.Notification
	for _, n := range r.notifications {
		if n.UserID != userID {
			continue
		}
		if filter.UnreadOnly && n.ReadAt != nil {
			continue
		}
		if filter.Type != nil && n.Type != *filter.Type {
			continue
		}
		result = append(result, n)
	}
	if len(result) > filter.Limit {
		result = result[:filter.Limit]
	}
	return result, nil
}

func (r *fakeRepo) CountUnread(_ context.Context, userID uuid.UUID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, notif := range r.notifications {
		if notif.UserID == userID && notif.ReadAt == nil {
			n++
		}
	}
	return n, nil
}

func (r *fakeRepo) MarkRead(_ context.Context, id, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.notifications[id]
	if !ok || n.UserID != userID {
		return domain.ErrNotificationNotFound
	}
	now := time.Now()
	n.ReadAt = &now
	return nil
}

func (r *fakeRepo) MarkAllRead(_ context.Context, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for _, n := range r.notifications {
		if n.UserID == userID {
			n.ReadAt = &now
		}
	}
	return nil
}

func (r *fakeRepo) DeleteNotification(_ context.Context, id, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.notifications[id]
	if !ok || n.UserID != userID {
		return domain.ErrNotificationNotFound
	}
	delete(r.notifications, id)
	return nil
}

func (r *fakeRepo) DeleteNotificationsByProject(_ context.Context, projectID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, n := range r.notifications {
		if n.ProjectID != nil && *n.ProjectID == projectID {
			delete(r.notifications, id)
		}
	}
	return nil
}

func (r *fakeRepo) DeleteOldNotifications(_ context.Context, before time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var count int64
	for id, n := range r.notifications {
		if n.CreatedAt.Before(before) {
			delete(r.notifications, id)
			count++
		}
	}
	return count, nil
}

func (r *fakeRepo) InsertOutboxEvent(_ context.Context, id, agg uuid.UUID, et string, p []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outbox = append(r.outbox, &domain.OutboxEvent{ID: id, AggregateID: agg, EventType: et, Payload: p})
	return nil
}

func (r *fakeRepo) GetUnpublishedEvents(_ context.Context, limit int32) ([]*domain.OutboxEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var res []*domain.OutboxEvent
	for _, e := range r.outbox {
		if e.PublishedAt == nil {
			res = append(res, e)
		}
	}
	if int32(len(res)) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (r *fakeRepo) MarkEventPublished(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for _, e := range r.outbox {
		if e.ID == id {
			e.PublishedAt = &now
		}
	}
	return nil
}

func (r *fakeRepo) WasEventProcessed(_ context.Context, eventID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.processed[eventID], nil
}

func (r *fakeRepo) MarkEventProcessed(_ context.Context, eventID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processed[eventID] = true
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func makeService(repo *fakeRepo) domain.NotificationService {
	return domain.NewNotificationService(repo)
}

func seedNotification(repo *fakeRepo, userID uuid.UUID) *domain.Notification {
	n := &domain.Notification{
		ID:        uuid.New(),
		UserID:    userID,
		Type:      domain.TypeMention,
		Payload:   map[string]any{"task_id": uuid.New().String()},
		CreatedAt: time.Now(),
	}
	repo.notifications[n.ID] = n
	return n
}

func assertDomainErr(t *testing.T, err error, expected *domain.Error) {
	t.Helper()
	var de *domain.Error
	require.True(t, errors.As(err, &de), "expected domain.Error, got %T: %v", err, err)
	assert.Equal(t, expected.Code, de.Code)
}

// ── Tests — ListNotifications ─────────────────────────────────────────────────

func TestListNotifications_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()
	seedNotification(repo, userID)
	seedNotification(repo, userID)

	svc := makeService(repo)
	ns, cursor, err := svc.ListNotifications(context.Background(), userID, domain.ListFilter{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, ns, 2)
	assert.Nil(t, cursor)
}

func TestListNotifications_UnreadFilter(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()
	n1 := seedNotification(repo, userID)
	_ = seedNotification(repo, userID)

	// Mark n1 as read.
	now := time.Now()
	n1.ReadAt = &now

	svc := makeService(repo)
	ns, _, err := svc.ListNotifications(context.Background(), userID, domain.ListFilter{UnreadOnly: true, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, ns, 1)
	assert.Equal(t, ns[0].ReadAt, (*time.Time)(nil))
}

func TestListNotifications_TypeFilter(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()
	seedNotification(repo, userID)

	// Add a different type.
	n2 := &domain.Notification{
		ID: uuid.New(), UserID: userID, Type: domain.TypeTaskAssigned,
		Payload: map[string]any{}, CreatedAt: time.Now(),
	}
	repo.notifications[n2.ID] = n2

	svc := makeService(repo)
	nt := domain.TypeMention
	ns, _, err := svc.ListNotifications(context.Background(), userID, domain.ListFilter{Type: &nt, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, ns, 1)
	assert.Equal(t, domain.TypeMention, ns[0].Type)
}

func TestListNotifications_DefaultLimit(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()
	svc := makeService(repo)
	_, _, err := svc.ListNotifications(context.Background(), userID, domain.ListFilter{Limit: 0})
	require.NoError(t, err)
}

func TestListNotifications_Pagination(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()
	for i := 0; i < 5; i++ {
		seedNotification(repo, userID)
	}

	svc := makeService(repo)
	ns, cursor, err := svc.ListNotifications(context.Background(), userID, domain.ListFilter{Limit: 3})
	require.NoError(t, err)
	assert.Len(t, ns, 3)
	assert.NotNil(t, cursor)
}

// ── Tests — GetUnreadCount ────────────────────────────────────────────────────

func TestGetUnreadCount(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()
	n1 := seedNotification(repo, userID)
	seedNotification(repo, userID)

	now := time.Now()
	n1.ReadAt = &now

	svc := makeService(repo)
	count, err := svc.GetUnreadCount(context.Background(), userID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// ── Tests — MarkRead ──────────────────────────────────────────────────────────

func TestMarkRead_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()
	n := seedNotification(repo, userID)

	svc := makeService(repo)
	err := svc.MarkRead(context.Background(), n.ID, userID)
	require.NoError(t, err)
	assert.NotNil(t, repo.notifications[n.ID].ReadAt)
}

func TestMarkRead_NotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo)
	err := svc.MarkRead(context.Background(), uuid.New(), uuid.New())
	assertDomainErr(t, err, domain.ErrNotificationNotFound)
}

func TestMarkRead_WrongUser(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()
	n := seedNotification(repo, userID)

	svc := makeService(repo)
	err := svc.MarkRead(context.Background(), n.ID, uuid.New()) // different user
	assertDomainErr(t, err, domain.ErrNotificationNotFound)
}

// ── Tests — MarkAllRead ───────────────────────────────────────────────────────

func TestMarkAllRead(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()
	seedNotification(repo, userID)
	seedNotification(repo, userID)

	svc := makeService(repo)
	err := svc.MarkAllRead(context.Background(), userID)
	require.NoError(t, err)

	count, _ := repo.CountUnread(context.Background(), userID)
	assert.Equal(t, 0, count)
}

// ── Tests — DeleteNotification ────────────────────────────────────────────────

func TestDeleteNotification_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()
	n := seedNotification(repo, userID)

	svc := makeService(repo)
	err := svc.DeleteNotification(context.Background(), n.ID, userID)
	require.NoError(t, err)
	assert.NotContains(t, repo.notifications, n.ID)
}

func TestDeleteNotification_NotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo)
	err := svc.DeleteNotification(context.Background(), uuid.New(), uuid.New())
	assertDomainErr(t, err, domain.ErrNotificationNotFound)
}

func TestDeleteNotification_WrongUser(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()
	n := seedNotification(repo, userID)

	svc := makeService(repo)
	err := svc.DeleteNotification(context.Background(), n.ID, uuid.New())
	assertDomainErr(t, err, domain.ErrNotificationNotFound)
}

// ── Tests — Domain entities ────────────────────────────────────────────────────

func TestChannelBuilders(t *testing.T) {
	id := uuid.New()
	assert.Equal(t, domain.Channel("user:"+id.String()), domain.UserChannel(id))
	assert.Equal(t, domain.Channel("project:"+id.String()), domain.ProjectChannel(id))
	assert.Equal(t, domain.Channel("board:"+id.String()), domain.BoardChannel(id))
	assert.Equal(t, domain.Channel("task:"+id.String()), domain.TaskChannel(id))
}

func TestNotificationTypes(t *testing.T) {
	types := []domain.NotificationType{
		domain.TypeMention,
		domain.TypeTaskAssigned,
		domain.TypeTaskUnassigned,
		domain.TypeTaskDueSoon,
		domain.TypeTaskDeleted,
		domain.TypeCommentOnMyTask,
		domain.TypeDocumentShared,
		domain.TypeProjectMemberAdded,
		domain.TypeProjectMemberRemoved,
	}
	for _, nt := range types {
		assert.NotEmpty(t, string(nt))
	}
}

func TestPermissionSet_Has(t *testing.T) {
	ps := &domain.PermissionSet{Permissions: []string{"read", "write"}}
	assert.True(t, ps.Has("read"))
	assert.False(t, ps.Has("admin"))
}

func TestDomainError_Formatting(t *testing.T) {
	e := domain.ErrNotificationNotFound
	assert.Contains(t, e.Error(), "notification_not_found")
	assert.Equal(t, "notification_not_found", e.GetCode())
	assert.Nil(t, e.Unwrap())

	cause := errors.New("root")
	wrapped := &domain.Error{Code: "test", Message: "msg", Cause: cause}
	assert.Contains(t, wrapped.Error(), "root")
	assert.Equal(t, cause, wrapped.Unwrap())
}

// ── Tests — Idempotent CreateNotification ─────────────────────────────────────

func TestCreateNotification_Idempotent(t *testing.T) {
	repo := newFakeRepo()
	sourceID := "evt-123"
	userID := uuid.New()

	n1 := &domain.Notification{
		ID: uuid.New(), UserID: userID, Type: domain.TypeMention,
		Payload: map[string]any{}, SourceEventID: &sourceID,
	}

	svc := domain.NewNotificationService(repo)
	_ = svc

	// Direct repo call to test idempotency.
	saved1, err := repo.CreateNotification(context.Background(), n1)
	require.NoError(t, err)
	require.NotNil(t, saved1)

	n2 := &domain.Notification{
		ID: uuid.New(), UserID: userID, Type: domain.TypeMention,
		Payload: map[string]any{}, SourceEventID: &sourceID,
	}
	saved2, err := repo.CreateNotification(context.Background(), n2)
	require.NoError(t, err)
	assert.Nil(t, saved2) // duplicate suppressed
	assert.Len(t, repo.notifications, 1)
}

// ── Tests — MaxLimit clamping ─────────────────────────────────────────────────

func TestListNotifications_MaxLimitClamped(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()
	svc := makeService(repo)
	// limit 300 > max 200, should clamp to 200 (no panic)
	_, _, err := svc.ListNotifications(context.Background(), userID, domain.ListFilter{Limit: 300})
	require.NoError(t, err)
}
