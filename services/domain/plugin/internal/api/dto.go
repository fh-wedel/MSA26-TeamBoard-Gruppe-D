package api

import (
	"time"

	"github.com/google/uuid"
	"github.com/teamboard/services/domain/plugin/internal/domain"
)

// ── Webhook DTOs ──────────────────────────────────────────────────────────────

type WebhookDTO struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	TargetURL   string    `json:"target_url"`
	Description string    `json:"description"`
	EventFilter []string  `json:"event_filter"`
	Active      bool      `json:"active"`
	CreatedBy   uuid.UUID `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type WebhookCreatedDTO struct {
	WebhookDTO
	Secret string `json:"secret"`
}

type CreateWebhookRequest struct {
	TargetURL   string   `json:"target_url"`
	Description string   `json:"description"`
	EventFilter []string `json:"event_filter"`
}

type UpdateWebhookRequest struct {
	TargetURL   *string   `json:"target_url"`
	Description *string   `json:"description"`
	EventFilter *[]string `json:"event_filter"`
	Active      *bool     `json:"active"`
}

// ── Delivery DTOs ─────────────────────────────────────────────────────────────

type DeliveryDTO struct {
	ID                  uuid.UUID       `json:"id"`
	WebhookID           uuid.UUID       `json:"webhook_id"`
	EventID             string          `json:"event_id"`
	EventType           string          `json:"event_type"`
	Status              string          `json:"status"`
	AttemptCount        int             `json:"attempt_count"`
	NextAttemptAt       *time.Time      `json:"next_attempt_at,omitempty"`
	LastResponseStatus  *int            `json:"last_response_status,omitempty"`
	LastResponseBody    *string         `json:"last_response_body,omitempty"`
	LastError           *string         `json:"last_error,omitempty"`
	LastAttemptedAt     *time.Time      `json:"last_attempted_at,omitempty"`
	DeliveredAt         *time.Time      `json:"delivered_at,omitempty"`
	FailedPermanentlyAt *time.Time      `json:"failed_permanently_at,omitempty"`
	DurationMs          *int            `json:"duration_ms,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
}

// ── Mappers ───────────────────────────────────────────────────────────────────

func toWebhookDTO(wh *domain.Webhook) WebhookDTO {
	return WebhookDTO{
		ID:          wh.ID,
		ProjectID:   wh.ProjectID,
		TargetURL:   wh.TargetURL,
		Description: wh.Description,
		EventFilter: wh.EventFilter,
		Active:      wh.Active,
		CreatedBy:   wh.CreatedBy,
		CreatedAt:   wh.CreatedAt,
		UpdatedAt:   wh.UpdatedAt,
	}
}

func toDeliveryDTO(d *domain.Delivery) DeliveryDTO {
	dto := DeliveryDTO{
		ID:                  d.ID,
		WebhookID:           d.WebhookID,
		EventID:             d.EventID,
		EventType:           d.EventType,
		Status:              string(d.Status),
		AttemptCount:        d.AttemptCount,
		LastResponseStatus:  d.LastResponseStatus,
		LastResponseBody:    d.LastResponseBody,
		LastError:           d.LastError,
		LastAttemptedAt:     d.LastAttemptedAt,
		DeliveredAt:         d.DeliveredAt,
		FailedPermanentlyAt: d.FailedPermanentlyAt,
		DurationMs:          d.DurationMs,
		CreatedAt:           d.CreatedAt,
	}
	if d.Status == domain.DeliveryStatusPending {
		dto.NextAttemptAt = &d.NextAttemptAt
	}
	return dto
}
