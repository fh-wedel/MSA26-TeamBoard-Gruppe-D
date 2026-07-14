package domain_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/teamboard/services/domain/plugin/internal/domain"
)

// ── Fake repository ───────────────────────────────────────────────────────────

type fakeRepo struct {
	mu            sync.Mutex
	webhooks      map[uuid.UUID]*domain.Webhook
	deliveries    map[uuid.UUID]*domain.Delivery
	outbox        []*domain.OutboxEvent
	processed     map[string]bool
	knownProjects map[uuid.UUID]bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		webhooks:      make(map[uuid.UUID]*domain.Webhook),
		deliveries:    make(map[uuid.UUID]*domain.Delivery),
		processed:     make(map[string]bool),
		knownProjects: make(map[uuid.UUID]bool),
	}
}

func (r *fakeRepo) CreateWebhook(_ context.Context, wh *domain.Webhook) (*domain.Webhook, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	wh.CreatedAt = time.Now(); wh.UpdatedAt = time.Now()
	r.webhooks[wh.ID] = wh
	return wh, nil
}

func (r *fakeRepo) GetWebhook(_ context.Context, id uuid.UUID) (*domain.Webhook, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	wh, ok := r.webhooks[id]
	if !ok { return nil, domain.ErrWebhookNotFound }
	return wh, nil
}

func (r *fakeRepo) ListWebhooksByProject(_ context.Context, projectID uuid.UUID) ([]*domain.Webhook, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var result []*domain.Webhook
	for _, wh := range r.webhooks {
		if wh.ProjectID == projectID { result = append(result, wh) }
	}
	return result, nil
}

func (r *fakeRepo) ListActiveWebhooksMatchingEvent(_ context.Context, projectID uuid.UUID, eventType string) ([]*domain.Webhook, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var result []*domain.Webhook
	for _, wh := range r.webhooks {
		if wh.ProjectID == projectID && wh.Active && domain.MatchesFilter(eventType, wh.EventFilter) {
			result = append(result, wh)
		}
	}
	return result, nil
}

func (r *fakeRepo) UpdateWebhook(_ context.Context, id uuid.UUID, patch domain.WebhookPatch) (*domain.Webhook, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	wh, ok := r.webhooks[id]
	if !ok { return nil, domain.ErrWebhookNotFound }
	if patch.TargetURL != nil { wh.TargetURL = *patch.TargetURL }
	if patch.Description != nil { wh.Description = *patch.Description }
	if patch.EventFilter != nil { wh.EventFilter = *patch.EventFilter }
	if patch.Active != nil { wh.Active = *patch.Active }
	wh.UpdatedAt = time.Now()
	return wh, nil
}

func (r *fakeRepo) UpdateWebhookSecret(_ context.Context, id uuid.UUID, secret string) (*domain.Webhook, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	wh, ok := r.webhooks[id]
	if !ok { return nil, domain.ErrWebhookNotFound }
	wh.Secret = secret
	return wh, nil
}

func (r *fakeRepo) SetWebhookActive(_ context.Context, id uuid.UUID, active bool) error {
	r.mu.Lock(); defer r.mu.Unlock()
	wh, ok := r.webhooks[id]
	if !ok { return domain.ErrWebhookNotFound }
	wh.Active = active
	return nil
}

func (r *fakeRepo) DeleteWebhook(_ context.Context, id uuid.UUID) error {
	r.mu.Lock(); defer r.mu.Unlock()
	if _, ok := r.webhooks[id]; !ok { return domain.ErrWebhookNotFound }
	delete(r.webhooks, id)
	return nil
}

func (r *fakeRepo) DeleteWebhooksByProject(_ context.Context, projectID uuid.UUID) error {
	r.mu.Lock(); defer r.mu.Unlock()
	for id, wh := range r.webhooks {
		if wh.ProjectID == projectID { delete(r.webhooks, id) }
	}
	return nil
}

func (r *fakeRepo) CreateDelivery(_ context.Context, d *domain.Delivery) (*domain.Delivery, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	d.CreatedAt = time.Now()
	r.deliveries[d.ID] = d
	return d, nil
}

func (r *fakeRepo) PickNextPendingDelivery(_ context.Context) (*domain.Delivery, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	now := time.Now()
	for _, d := range r.deliveries {
		if d.Status == domain.DeliveryStatusPending && !d.NextAttemptAt.After(now) {
			return d, nil
		}
	}
	return nil, domain.ErrNoPendingDelivery
}

func (r *fakeRepo) GetDelivery(_ context.Context, id uuid.UUID) (*domain.Delivery, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	d, ok := r.deliveries[id]
	if !ok { return nil, domain.ErrDeliveryNotFound }
	return d, nil
}

func (r *fakeRepo) ListDeliveriesByWebhook(_ context.Context, webhookID uuid.UUID, filter domain.DeliveryFilter) ([]*domain.Delivery, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var result []*domain.Delivery
	for _, d := range r.deliveries {
		if d.WebhookID != webhookID { continue }
		if filter.Status != nil && d.Status != *filter.Status { continue }
		if filter.EventType != nil && d.EventType != *filter.EventType { continue }
		result = append(result, d)
	}
	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}
	return result, nil
}

func (r *fakeRepo) MarkDeliveryDelivered(_ context.Context, id uuid.UUID, statusCode int, body string, durationMs int) error {
	r.mu.Lock(); defer r.mu.Unlock()
	d, ok := r.deliveries[id]
	if !ok { return domain.ErrDeliveryNotFound }
	d.Status = domain.DeliveryStatusDelivered
	d.AttemptCount++
	d.LastResponseStatus = &statusCode
	d.LastResponseBody = &body
	d.DurationMs = &durationMs
	now := time.Now()
	d.DeliveredAt = &now
	d.LastAttemptedAt = &now
	return nil
}

func (r *fakeRepo) RescheduleDelivery(_ context.Context, id uuid.UUID, nextAt time.Time, statusCode *int, body, errMsg *string, durationMs int) error {
	r.mu.Lock(); defer r.mu.Unlock()
	d, ok := r.deliveries[id]
	if !ok { return domain.ErrDeliveryNotFound }
	d.Status = domain.DeliveryStatusPending
	d.NextAttemptAt = nextAt
	d.AttemptCount++
	d.LastResponseStatus = statusCode
	d.LastResponseBody = body
	d.LastError = errMsg
	d.DurationMs = &durationMs
	now := time.Now()
	d.LastAttemptedAt = &now
	return nil
}

func (r *fakeRepo) MarkDeliveryDead(_ context.Context, id uuid.UUID, reason string) error {
	r.mu.Lock(); defer r.mu.Unlock()
	d, ok := r.deliveries[id]
	if !ok { return domain.ErrDeliveryNotFound }
	d.Status = domain.DeliveryStatusDead
	d.AttemptCount++
	d.LastError = &reason
	now := time.Now()
	d.FailedPermanentlyAt = &now
	d.LastAttemptedAt = &now
	return nil
}

func (r *fakeRepo) MarkPendingDeliveriesDeadByWebhook(_ context.Context, webhookID uuid.UUID) error {
	r.mu.Lock(); defer r.mu.Unlock()
	for _, d := range r.deliveries {
		if d.WebhookID == webhookID && d.Status == domain.DeliveryStatusPending {
			d.Status = domain.DeliveryStatusDead
		}
	}
	return nil
}

func (r *fakeRepo) DeleteOldDeliveries(_ context.Context, before time.Time) (int64, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var count int64
	for id, d := range r.deliveries {
		if d.CreatedAt.Before(before) && (d.Status == domain.DeliveryStatusDelivered || d.Status == domain.DeliveryStatusDead) {
			delete(r.deliveries, id)
			count++
		}
	}
	return count, nil
}

func (r *fakeRepo) UpsertKnownProject(_ context.Context, id uuid.UUID) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.knownProjects[id] = true
	return nil
}

func (r *fakeRepo) KnownProjectExists(_ context.Context, id uuid.UUID) (bool, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	return r.knownProjects[id], nil
}

func (r *fakeRepo) MarkKnownProjectDeleted(_ context.Context, id uuid.UUID) error {
	r.mu.Lock(); defer r.mu.Unlock()
	delete(r.knownProjects, id)
	return nil
}

func (r *fakeRepo) InsertOutboxEvent(_ context.Context, id, agg uuid.UUID, et string, p []byte) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.outbox = append(r.outbox, &domain.OutboxEvent{ID: id, AggregateID: agg, EventType: et, Payload: p})
	return nil
}

func (r *fakeRepo) GetUnpublishedEvents(_ context.Context, limit int32) ([]*domain.OutboxEvent, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var result []*domain.OutboxEvent
	for _, e := range r.outbox {
		if e.PublishedAt == nil { result = append(result, e) }
	}
	if int32(len(result)) > limit { result = result[:limit] }
	return result, nil
}

func (r *fakeRepo) MarkEventPublished(_ context.Context, id uuid.UUID) error {
	r.mu.Lock(); defer r.mu.Unlock()
	now := time.Now()
	for _, e := range r.outbox {
		if e.ID == id { e.PublishedAt = &now }
	}
	return nil
}

func (r *fakeRepo) WasEventProcessed(_ context.Context, eventID string) (bool, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	return r.processed[eventID], nil
}

func (r *fakeRepo) MarkEventProcessed(_ context.Context, eventID string) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.processed[eventID] = true
	return nil
}

// ── Fake permission checker ───────────────────────────────────────────────────

type fakePerms struct{ allow bool }

func (p *fakePerms) HasPermission(_ context.Context, _, _ uuid.UUID, _ string) (bool, error) {
	return p.allow, nil
}

// ── Helper ────────────────────────────────────────────────────────────────────

func makeService(repo *fakeRepo, allow bool) domain.WebhookService {
	return domain.NewWebhookService(repo, &fakePerms{allow: allow}, true, true, nil)
}

func seedWebhook(repo *fakeRepo, projectID uuid.UUID) *domain.Webhook {
	wh := &domain.Webhook{
		ID:          uuid.New(),
		ProjectID:   projectID,
		TargetURL:   "https://example.com/hook",
		EventFilter: []string{"task.*"},
		Active:      true,
		Secret:      "topsecret",
		CreatedBy:   uuid.New(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	repo.webhooks[wh.ID] = wh
	return wh
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestCreateWebhook_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo, true)
	projectID := uuid.New()
	wh, secret, err := svc.CreateWebhook(context.Background(), uuid.New(), domain.CreateWebhookInput{
		ProjectID:   projectID,
		TargetURL:   "https://example.com/hook",
		EventFilter: []string{"task.created"},
	})
	require.NoError(t, err)
	assert.NotNil(t, wh)
	assert.NotEmpty(t, secret)
	assert.Equal(t, projectID, wh.ProjectID)
	assert.Equal(t, "", wh.Secret) // secret not returned in webhook struct
}

func TestCreateWebhook_PermissionDenied(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo, false)
	_, _, err := svc.CreateWebhook(context.Background(), uuid.New(), domain.CreateWebhookInput{
		ProjectID:   uuid.New(),
		TargetURL:   "https://example.com/hook",
		EventFilter: []string{"task.created"},
	})
	var de *domain.Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "permission_denied", de.Code)
}

func TestCreateWebhook_InvalidEventFilter(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo, true)
	_, _, err := svc.CreateWebhook(context.Background(), uuid.New(), domain.CreateWebhookInput{
		ProjectID:   uuid.New(),
		TargetURL:   "https://example.com/hook",
		EventFilter: []string{},
	})
	require.Error(t, err)
}

func TestGetWebhook_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	svc := makeService(repo, true)
	got, err := svc.GetWebhook(context.Background(), wh.ID, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, wh.ID, got.ID)
	assert.Empty(t, got.Secret)
}

func TestGetWebhook_NotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo, true)
	_, err := svc.GetWebhook(context.Background(), uuid.New(), uuid.New())
	var de *domain.Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "webhook_not_found", de.Code)
}

func TestListWebhooks(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	seedWebhook(repo, projectID)
	seedWebhook(repo, projectID)
	seedWebhook(repo, uuid.New()) // different project
	svc := makeService(repo, true)
	whs, err := svc.ListWebhooks(context.Background(), projectID, uuid.New())
	require.NoError(t, err)
	assert.Len(t, whs, 2)
	for _, wh := range whs {
		assert.Empty(t, wh.Secret)
	}
}

func TestUpdateWebhook_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	svc := makeService(repo, true)
	newURL := "https://example.com/new-hook"
	updated, err := svc.UpdateWebhook(context.Background(), wh.ID, uuid.New(), domain.WebhookPatch{
		TargetURL: &newURL,
	})
	require.NoError(t, err)
	assert.Equal(t, newURL, updated.TargetURL)
}

func TestDeleteWebhook_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	svc := makeService(repo, true)
	err := svc.DeleteWebhook(context.Background(), wh.ID, uuid.New())
	require.NoError(t, err)
	assert.NotContains(t, repo.webhooks, wh.ID)
}

func TestRotateSecret(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	oldSecret := wh.Secret
	svc := makeService(repo, true)
	newSecret, err := svc.RotateSecret(context.Background(), wh.ID, uuid.New())
	require.NoError(t, err)
	assert.NotEmpty(t, newSecret)
	assert.NotEqual(t, oldSecret, newSecret)
}

func TestEnableDisableWebhook(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	wh.Active = true
	svc := makeService(repo, true)

	err := svc.DisableWebhook(context.Background(), wh.ID, uuid.New())
	require.NoError(t, err)
	assert.False(t, repo.webhooks[wh.ID].Active)

	err = svc.EnableWebhook(context.Background(), wh.ID, uuid.New())
	require.NoError(t, err)
	assert.True(t, repo.webhooks[wh.ID].Active)
}

func TestTriggerTest(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	svc := makeService(repo, true)
	d, err := svc.TriggerTest(context.Background(), wh.ID, uuid.New(), "webhook.test")
	require.NoError(t, err)
	assert.NotNil(t, d)
	assert.Equal(t, "webhook.test", d.EventType)
}

func TestListDeliveries(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	for i := 0; i < 3; i++ {
		d := &domain.Delivery{
			ID: uuid.New(), WebhookID: wh.ID, EventID: uuid.New().String(),
			EventType: "task.created", Status: domain.DeliveryStatusPending,
			CreatedAt: time.Now(),
		}
		repo.deliveries[d.ID] = d
	}
	svc := makeService(repo, true)
	deliveries, cursor, err := svc.ListDeliveries(context.Background(), wh.ID, uuid.New(), domain.DeliveryFilter{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, deliveries, 3)
	assert.Nil(t, cursor)
}

func TestListDeliveries_Pagination(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	for i := 0; i < 5; i++ {
		d := &domain.Delivery{
			ID: uuid.New(), WebhookID: wh.ID, EventID: uuid.New().String(),
			EventType: "task.created", Status: domain.DeliveryStatusPending,
			CreatedAt: time.Now(),
		}
		repo.deliveries[d.ID] = d
	}
	svc := makeService(repo, true)
	deliveries, cursor, err := svc.ListDeliveries(context.Background(), wh.ID, uuid.New(), domain.DeliveryFilter{Limit: 3})
	require.NoError(t, err)
	assert.Len(t, deliveries, 3)
	assert.NotNil(t, cursor)
}

func TestDispatcher_EnqueueForEvent(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	repo.knownProjects[projectID] = true

	wh := &domain.Webhook{
		ID: uuid.New(), ProjectID: projectID,
		TargetURL:   "https://example.com/hook",
		EventFilter: []string{"task.*"},
		Active:      true,
		Secret:      "s",
		CreatedAt:   time.Now(), UpdatedAt: time.Now(),
	}
	repo.webhooks[wh.ID] = wh

	dispatcher := domain.NewDispatcherService(repo)
	env := domain.Envelope{
		EventID:   "evt-1",
		EventType: "task.created",
		AggregateID: uuid.New(),
		Payload:   map[string]any{"project_id": projectID.String()},
	}
	err := dispatcher.EnqueueForEvent(context.Background(), env)
	require.NoError(t, err)
	assert.Len(t, repo.deliveries, 1)
	assert.True(t, repo.processed["evt-1"])
}

func TestDispatcher_Idempotent(t *testing.T) {
	repo := newFakeRepo()
	repo.processed["evt-dup"] = true
	dispatcher := domain.NewDispatcherService(repo)
	env := domain.Envelope{EventID: "evt-dup", EventType: "task.created", Payload: map[string]any{}}
	err := dispatcher.EnqueueForEvent(context.Background(), env)
	require.NoError(t, err)
	assert.Len(t, repo.deliveries, 0) // no deliveries created for duplicate
}

func TestDispatcher_UnknownProject(t *testing.T) {
	repo := newFakeRepo()
	dispatcher := domain.NewDispatcherService(repo)
	env := domain.Envelope{
		EventID:   "evt-x",
		EventType: "task.created",
		Payload:   map[string]any{"project_id": uuid.New().String()},
	}
	err := dispatcher.EnqueueForEvent(context.Background(), env)
	require.NoError(t, err)
	assert.Len(t, repo.deliveries, 0)
}
