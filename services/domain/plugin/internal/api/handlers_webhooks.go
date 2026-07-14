package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/teamboard/services/domain/plugin/internal/domain"
)

type webhookHandlers struct {
	svc domain.WebhookService
}

func (h *webhookHandlers) listWebhooks(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid project id"))
		return
	}
	requester := mustUserID(r)
	whs, err := h.svc.ListWebhooks(r.Context(), projectID, requester)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	dtos := make([]WebhookDTO, len(whs))
	for i, wh := range whs {
		dtos[i] = toWebhookDTO(wh)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": dtos})
}

func (h *webhookHandlers) createWebhook(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid project id"))
		return
	}
	var req CreateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid request body"))
		return
	}
	if req.TargetURL == "" || len(req.EventFilter) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, errResp("target_url and event_filter are required"))
		return
	}
	requester := mustUserID(r)
	wh, secret, err := h.svc.CreateWebhook(r.Context(), requester, domain.CreateWebhookInput{
		ProjectID:   projectID,
		TargetURL:   req.TargetURL,
		Description: req.Description,
		EventFilter: req.EventFilter,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	dto := WebhookCreatedDTO{
		WebhookDTO: toWebhookDTO(wh),
		Secret:     secret,
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": dto})
}

func (h *webhookHandlers) getWebhook(w http.ResponseWriter, r *http.Request) {
	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid webhook id"))
		return
	}
	requester := mustUserID(r)
	wh, err := h.svc.GetWebhook(r.Context(), webhookID, requester)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toWebhookDTO(wh)})
}

func (h *webhookHandlers) updateWebhook(w http.ResponseWriter, r *http.Request) {
	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid webhook id"))
		return
	}
	var req UpdateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid request body"))
		return
	}
	requester := mustUserID(r)
	wh, err := h.svc.UpdateWebhook(r.Context(), webhookID, requester, domain.WebhookPatch{
		TargetURL:   req.TargetURL,
		Description: req.Description,
		EventFilter: req.EventFilter,
		Active:      req.Active,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toWebhookDTO(wh)})
}

func (h *webhookHandlers) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid webhook id"))
		return
	}
	requester := mustUserID(r)
	if err := h.svc.DeleteWebhook(r.Context(), webhookID, requester); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *webhookHandlers) rotateSecret(w http.ResponseWriter, r *http.Request) {
	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid webhook id"))
		return
	}
	requester := mustUserID(r)
	secret, err := h.svc.RotateSecret(r.Context(), webhookID, requester)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"secret": secret}})
}

func (h *webhookHandlers) enableWebhook(w http.ResponseWriter, r *http.Request) {
	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid webhook id"))
		return
	}
	requester := mustUserID(r)
	if err := h.svc.EnableWebhook(r.Context(), webhookID, requester); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *webhookHandlers) disableWebhook(w http.ResponseWriter, r *http.Request) {
	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid webhook id"))
		return
	}
	requester := mustUserID(r)
	if err := h.svc.DisableWebhook(r.Context(), webhookID, requester); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *webhookHandlers) triggerTest(w http.ResponseWriter, r *http.Request) {
	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid webhook id"))
		return
	}
	var req struct {
		EventType string `json:"event_type"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	requester := mustUserID(r)
	d, err := h.svc.TriggerTest(r.Context(), webhookID, requester, req.EventType)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"data": map[string]any{"delivery_id": d.ID}})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errResp(msg string) map[string]any {
	return map[string]any{"error": msg}
}

func writeDomainError(w http.ResponseWriter, err error) {
	var de *domain.Error
	if errors.As(err, &de) {
		status := domainErrStatus(de.Code)
		writeJSON(w, status, map[string]any{"error": de.Code, "message": de.Message})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
}

func domainErrStatus(code string) int {
	switch code {
	case domain.ErrWebhookNotFound.Code, domain.ErrDeliveryNotFound.Code:
		return http.StatusNotFound
	case domain.ErrPermissionDenied.Code:
		return http.StatusForbidden
	case domain.ErrInvalidURL.Code, domain.ErrPrivateURLForbidden.Code,
		domain.ErrInvalidEventFilter.Code, domain.ErrValidation.Code:
		return http.StatusUnprocessableEntity
	case domain.ErrProjectUnknown.Code:
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
