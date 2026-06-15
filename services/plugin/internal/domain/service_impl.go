package domain

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	maxDeliveryLimit    = 200
	defaultDeliveryLimit = 50
)

type svc struct {
	repo        Repository
	perms       PermissionChecker
	allowHTTP   bool
	allowPrivate bool
	allowedPorts []int
}

func NewWebhookService(repo Repository, perms PermissionChecker, allowHTTP, allowPrivate bool, allowedPorts []int) WebhookService {
	return &svc{
		repo:         repo,
		perms:        perms,
		allowHTTP:    allowHTTP,
		allowPrivate: allowPrivate,
		allowedPorts: allowedPorts,
	}
}

func (s *svc) checkManage(ctx context.Context, projectID, userID uuid.UUID) error {
	ok, err := s.perms.HasPermission(ctx, projectID, userID, "webhook:manage")
	if err != nil {
		return err
	}
	if !ok {
		return ErrPermissionDenied
	}
	return nil
}

func (s *svc) CreateWebhook(ctx context.Context, requester uuid.UUID, input CreateWebhookInput) (*Webhook, string, error) {
	if err := s.checkManage(ctx, input.ProjectID, requester); err != nil {
		return nil, "", err
	}
	if err := ValidateWebhookURL(input.TargetURL, s.allowHTTP, s.allowPrivate, s.allowedPorts); err != nil {
		return nil, "", err
	}
	if err := ValidateEventFilter(input.EventFilter); err != nil {
		return nil, "", err
	}

	plain, err := generateSecret()
	if err != nil {
		return nil, "", err
	}

	wh := &Webhook{
		ID:          uuid.New(),
		ProjectID:   input.ProjectID,
		TargetURL:   input.TargetURL,
		Description: input.Description,
		Secret:      plain,
		EventFilter: input.EventFilter,
		Active:      true,
		CreatedBy:   requester,
	}
	saved, err := s.repo.CreateWebhook(ctx, wh)
	if err != nil {
		return nil, "", err
	}

	payload, _ := json.Marshal(map[string]any{
		"webhook_id":   saved.ID,
		"project_id":   saved.ProjectID,
		"target_url":   saved.TargetURL,
		"event_filter": saved.EventFilter,
		"created_by":   saved.CreatedBy,
	})
	_ = s.repo.InsertOutboxEvent(ctx, uuid.New(), saved.ProjectID, "webhook.created", payload)

	saved.Secret = "" // secret only returned via the second return value
	return saved, plain, nil
}

func (s *svc) GetWebhook(ctx context.Context, webhookID, requester uuid.UUID) (*Webhook, error) {
	wh, err := s.repo.GetWebhook(ctx, webhookID)
	if err != nil {
		return nil, err
	}
	if err := s.checkManage(ctx, wh.ProjectID, requester); err != nil {
		return nil, err
	}
	wh.Secret = "" // never expose
	return wh, nil
}

func (s *svc) ListWebhooks(ctx context.Context, projectID, requester uuid.UUID) ([]*Webhook, error) {
	if err := s.checkManage(ctx, projectID, requester); err != nil {
		return nil, err
	}
	whs, err := s.repo.ListWebhooksByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, wh := range whs {
		wh.Secret = ""
	}
	return whs, nil
}

func (s *svc) UpdateWebhook(ctx context.Context, webhookID, requester uuid.UUID, patch WebhookPatch) (*Webhook, error) {
	wh, err := s.repo.GetWebhook(ctx, webhookID)
	if err != nil {
		return nil, err
	}
	if err := s.checkManage(ctx, wh.ProjectID, requester); err != nil {
		return nil, err
	}
	if patch.TargetURL != nil {
		if err := ValidateWebhookURL(*patch.TargetURL, s.allowHTTP, s.allowPrivate, s.allowedPorts); err != nil {
			return nil, err
		}
	}
	if patch.EventFilter != nil {
		if err := ValidateEventFilter(*patch.EventFilter); err != nil {
			return nil, err
		}
	}
	updated, err := s.repo.UpdateWebhook(ctx, webhookID, patch)
	if err != nil {
		return nil, err
	}
	updated.Secret = ""
	return updated, nil
}

func (s *svc) RotateSecret(ctx context.Context, webhookID, requester uuid.UUID) (string, error) {
	wh, err := s.repo.GetWebhook(ctx, webhookID)
	if err != nil {
		return "", err
	}
	if err := s.checkManage(ctx, wh.ProjectID, requester); err != nil {
		return "", err
	}
	plain, err := generateSecret()
	if err != nil {
		return "", err
	}
	if _, err := s.repo.UpdateWebhookSecret(ctx, webhookID, plain); err != nil {
		return "", err
	}
	return plain, nil
}

func (s *svc) EnableWebhook(ctx context.Context, webhookID, requester uuid.UUID) error {
	return s.setActive(ctx, webhookID, requester, true)
}

func (s *svc) DisableWebhook(ctx context.Context, webhookID, requester uuid.UUID) error {
	return s.setActive(ctx, webhookID, requester, false)
}

func (s *svc) setActive(ctx context.Context, webhookID, requester uuid.UUID, active bool) error {
	wh, err := s.repo.GetWebhook(ctx, webhookID)
	if err != nil {
		return err
	}
	if err := s.checkManage(ctx, wh.ProjectID, requester); err != nil {
		return err
	}
	return s.repo.SetWebhookActive(ctx, webhookID, active)
}

func (s *svc) DeleteWebhook(ctx context.Context, webhookID, requester uuid.UUID) error {
	wh, err := s.repo.GetWebhook(ctx, webhookID)
	if err != nil {
		return err
	}
	if err := s.checkManage(ctx, wh.ProjectID, requester); err != nil {
		return err
	}
	if err := s.repo.MarkPendingDeliveriesDeadByWebhook(ctx, webhookID); err != nil {
		return err
	}
	if err := s.repo.DeleteWebhook(ctx, webhookID); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"webhook_id": webhookID,
		"project_id": wh.ProjectID,
		"deleted_by": requester,
	})
	_ = s.repo.InsertOutboxEvent(ctx, uuid.New(), wh.ProjectID, "webhook.deleted", payload)
	return nil
}

func (s *svc) TriggerTest(ctx context.Context, webhookID, requester uuid.UUID, eventType string) (*Delivery, error) {
	wh, err := s.repo.GetWebhook(ctx, webhookID)
	if err != nil {
		return nil, err
	}
	if err := s.checkManage(ctx, wh.ProjectID, requester); err != nil {
		return nil, err
	}
	if eventType == "" {
		eventType = "webhook.test"
	}
	d := &Delivery{
		ID:        uuid.New(),
		WebhookID: webhookID,
		EventID:   "test-" + uuid.New().String(),
		EventType: eventType,
		Payload: map[string]any{
			"event_type": eventType,
			"test":       true,
		},
		Status:        DeliveryStatusPending,
		NextAttemptAt: time.Now(),
	}
	return s.repo.CreateDelivery(ctx, d)
}

func (s *svc) ListDeliveries(ctx context.Context, webhookID, requester uuid.UUID, filter DeliveryFilter) ([]*Delivery, *Cursor, error) {
	wh, err := s.repo.GetWebhook(ctx, webhookID)
	if err != nil {
		return nil, nil, err
	}
	if err := s.checkManage(ctx, wh.ProjectID, requester); err != nil {
		return nil, nil, err
	}

	if filter.Limit <= 0 {
		filter.Limit = defaultDeliveryLimit
	}
	if filter.Limit > maxDeliveryLimit {
		filter.Limit = maxDeliveryLimit
	}

	if filter.Cursor != nil {
		decoded, err := base64.StdEncoding.DecodeString(*filter.Cursor)
		if err != nil {
			return nil, nil, &Error{Code: ErrValidation.Code, Message: "invalid cursor"}
		}
		var c Cursor
		if err := json.Unmarshal(decoded, &c); err != nil {
			return nil, nil, &Error{Code: ErrValidation.Code, Message: "invalid cursor"}
		}
		encoded := c.CreatedAt.Format(time.RFC3339Nano) + "," + c.ID.String()
		filter.Cursor = &encoded
	}

	deliveries, err := s.repo.ListDeliveriesByWebhook(ctx, webhookID, filter)
	if err != nil {
		return nil, nil, err
	}

	var cursor *Cursor
	if len(deliveries) == filter.Limit {
		last := deliveries[len(deliveries)-1]
		cursor = &Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return deliveries, cursor, nil
}

func (s *svc) GetDelivery(ctx context.Context, deliveryID, requester uuid.UUID) (*Delivery, error) {
	d, err := s.repo.GetDelivery(ctx, deliveryID)
	if err != nil {
		return nil, err
	}
	wh, err := s.repo.GetWebhook(ctx, d.WebhookID)
	if err != nil {
		return nil, err
	}
	if err := s.checkManage(ctx, wh.ProjectID, requester); err != nil {
		return nil, err
	}
	return d, nil
}

// ── Dispatcher ────────────────────────────────────────────────────────────────

type dispatcher struct {
	repo Repository
}

func NewDispatcherService(repo Repository) DispatcherService {
	return &dispatcher{repo: repo}
}

func (d *dispatcher) EnqueueForEvent(ctx context.Context, env Envelope) error {
	// Idempotency check
	processed, err := d.repo.WasEventProcessed(ctx, env.EventID)
	if err != nil {
		return err
	}
	if processed {
		return nil
	}

	projectID, ok := extractProjectID(env)
	if !ok {
		return d.repo.MarkEventProcessed(ctx, env.EventID)
	}

	exists, err := d.repo.KnownProjectExists(ctx, projectID)
	if err != nil {
		return err
	}
	if !exists {
		return d.repo.MarkEventProcessed(ctx, env.EventID)
	}

	webhooks, err := d.repo.ListActiveWebhooksMatchingEvent(ctx, projectID, env.EventType)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}
	rawPayload := make(map[string]any)
	if err := json.Unmarshal(payload, &rawPayload); err != nil {
		return err
	}

	for _, wh := range webhooks {
		delivery := &Delivery{
			ID:            uuid.New(),
			WebhookID:     wh.ID,
			EventID:       env.EventID,
			EventType:     env.EventType,
			Payload:       rawPayload,
			Status:        DeliveryStatusPending,
			NextAttemptAt: time.Now(),
		}
		if _, err := d.repo.CreateDelivery(ctx, delivery); err != nil {
			return err
		}
	}

	return d.repo.MarkEventProcessed(ctx, env.EventID)
}

// extractProjectID extracts the project UUID from an event envelope based on event type family.
func extractProjectID(env Envelope) (uuid.UUID, bool) {
	switch {
	case hasPrefix(env.EventType, "project."):
		return env.AggregateID, true
	case hasPrefix(env.EventType, "task."),
		hasPrefix(env.EventType, "board."),
		hasPrefix(env.EventType, "document."):
		if pidStr, ok := env.Payload["project_id"].(string); ok {
			id, err := uuid.Parse(pidStr)
			if err == nil {
				return id, true
			}
		}
	}
	return uuid.Nil, false
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
