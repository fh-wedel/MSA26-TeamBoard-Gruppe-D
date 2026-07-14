package domain_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/teamboard/services/domain/plugin/internal/domain"
)

// ── Error type tests ───────────────────────────────────────────────────────────

func TestDomainError_Formatting(t *testing.T) {
	e := domain.ErrWebhookNotFound
	assert.Contains(t, e.Error(), "webhook_not_found")
	assert.Equal(t, "webhook_not_found", e.GetCode())
	assert.Nil(t, e.Unwrap())

	cause := errors.New("root cause")
	wrapped := &domain.Error{Code: "test", Message: "msg", Cause: cause}
	assert.Contains(t, wrapped.Error(), "root cause")
	assert.Equal(t, cause, wrapped.Unwrap())
}

func TestDomainError_NoMessage(t *testing.T) {
	e := &domain.Error{Code: "just_code"}
	assert.Equal(t, "just_code", e.Error())
}

// ── Permission-denied paths ────────────────────────────────────────────────────

func TestGetWebhook_PermissionDenied(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	svc := makeService(repo, false) // perms denied
	_, err := svc.GetWebhook(context.Background(), wh.ID, uuid.New())
	var de *domain.Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "permission_denied", de.Code)
}

func TestListWebhooks_PermissionDenied(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo, false)
	_, err := svc.ListWebhooks(context.Background(), uuid.New(), uuid.New())
	var de *domain.Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "permission_denied", de.Code)
}

func TestUpdateWebhook_NotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo, true)
	_, err := svc.UpdateWebhook(context.Background(), uuid.New(), uuid.New(), domain.WebhookPatch{})
	var de *domain.Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "webhook_not_found", de.Code)
}

func TestUpdateWebhook_InvalidURL(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	svc := domain.NewWebhookService(repo, &fakePerms{allow: true}, false, false, nil)
	badURL := "ftp://bad.example.com"
	_, err := svc.UpdateWebhook(context.Background(), wh.ID, uuid.New(), domain.WebhookPatch{TargetURL: &badURL})
	var de *domain.Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "invalid_url", de.Code)
}

func TestRotateSecret_NotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo, true)
	_, err := svc.RotateSecret(context.Background(), uuid.New(), uuid.New())
	var de *domain.Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "webhook_not_found", de.Code)
}

func TestDeleteWebhook_NotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo, true)
	err := svc.DeleteWebhook(context.Background(), uuid.New(), uuid.New())
	var de *domain.Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "webhook_not_found", de.Code)
}

func TestDeleteWebhook_PermissionDenied(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	svc := makeService(repo, false)
	err := svc.DeleteWebhook(context.Background(), wh.ID, uuid.New())
	var de *domain.Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "permission_denied", de.Code)
}

func TestDeleteWebhook_MarksPendingDeliveriesDead(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	d := &domain.Delivery{
		ID: uuid.New(), WebhookID: wh.ID, EventType: "task.created",
		Status: domain.DeliveryStatusPending,
	}
	repo.deliveries[d.ID] = d

	svc := makeService(repo, true)
	err := svc.DeleteWebhook(context.Background(), wh.ID, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, domain.DeliveryStatusDead, repo.deliveries[d.ID].Status)
}

func TestTriggerTest_DefaultEventType(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	svc := makeService(repo, true)
	d, err := svc.TriggerTest(context.Background(), wh.ID, uuid.New(), "")
	require.NoError(t, err)
	assert.Equal(t, "webhook.test", d.EventType)
}

func TestGetDelivery_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	delivery := &domain.Delivery{
		ID: uuid.New(), WebhookID: wh.ID, EventType: "task.created",
		Status: domain.DeliveryStatusDelivered,
	}
	repo.deliveries[delivery.ID] = delivery

	svc := makeService(repo, true)
	got, err := svc.GetDelivery(context.Background(), delivery.ID, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, delivery.ID, got.ID)
}

func TestGetDelivery_NotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo, true)
	_, err := svc.GetDelivery(context.Background(), uuid.New(), uuid.New())
	var de *domain.Error
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "delivery_not_found", de.Code)
}

func TestListDeliveries_DefaultLimit(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	svc := makeService(repo, true)
	_, _, err := svc.ListDeliveries(context.Background(), wh.ID, uuid.New(), domain.DeliveryFilter{Limit: 0})
	require.NoError(t, err)
}

func TestListDeliveries_MaxLimitClamped(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	svc := makeService(repo, true)
	// limit 300 > max 200 — should not panic
	_, _, err := svc.ListDeliveries(context.Background(), wh.ID, uuid.New(), domain.DeliveryFilter{Limit: 300})
	require.NoError(t, err)
}

func TestDispatcher_ProjectEventUsesAggregateID(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	repo.knownProjects[projectID] = true

	wh := &domain.Webhook{
		ID: uuid.New(), ProjectID: projectID,
		TargetURL: "https://example.com/hook", EventFilter: []string{"project.*"},
		Active: true, Secret: "s",
	}
	repo.webhooks[wh.ID] = wh

	dispatcher := domain.NewDispatcherService(repo)
	env := domain.Envelope{
		EventID:     "evt-proj",
		EventType:   "project.updated",
		AggregateID: projectID,
		Payload:     map[string]any{},
	}
	err := dispatcher.EnqueueForEvent(context.Background(), env)
	require.NoError(t, err)
	assert.Len(t, repo.deliveries, 1)
}

func TestDispatcher_UserEventSkipped(t *testing.T) {
	repo := newFakeRepo()
	dispatcher := domain.NewDispatcherService(repo)
	env := domain.Envelope{
		EventID:   "evt-user",
		EventType: "user.deleted",
		Payload:   map[string]any{},
	}
	err := dispatcher.EnqueueForEvent(context.Background(), env)
	require.NoError(t, err)
	assert.Len(t, repo.deliveries, 0)
}

func TestListDeliveries_InvalidCursor(t *testing.T) {
	repo := newFakeRepo()
	projectID := uuid.New()
	wh := seedWebhook(repo, projectID)
	svc := makeService(repo, true)
	bad := "not-valid-base64!!!"
	_, _, err := svc.ListDeliveries(context.Background(), wh.ID, uuid.New(), domain.DeliveryFilter{
		Limit:  10,
		Cursor: &bad,
	})
	var de *domain.Error
	require.True(t, errors.As(err, &de))
}
