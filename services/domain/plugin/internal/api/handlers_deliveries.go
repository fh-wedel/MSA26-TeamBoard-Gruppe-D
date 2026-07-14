package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/teamboard/services/domain/plugin/internal/domain"
)

type deliveryHandlers struct {
	svc domain.WebhookService
}

func (h *deliveryHandlers) listDeliveries(w http.ResponseWriter, r *http.Request) {
	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid webhook id"))
		return
	}

	filter := domain.DeliveryFilter{}

	if s := r.URL.Query().Get("status"); s != "" {
		ds := domain.DeliveryStatus(s)
		filter.Status = &ds
	}
	if et := r.URL.Query().Get("event_type"); et != "" {
		filter.EventType = &et
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			filter.Limit = n
		}
	}
	if c := r.URL.Query().Get("cursor"); c != "" {
		filter.Cursor = &c
	}

	requester := mustUserID(r)
	deliveries, cursor, err := h.svc.ListDeliveries(r.Context(), webhookID, requester, filter)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	dtos := make([]DeliveryDTO, len(deliveries))
	for i, d := range deliveries {
		dtos[i] = toDeliveryDTO(d)
	}

	var nextCursor *string
	if cursor != nil {
		raw, _ := json.Marshal(cursor)
		enc := base64.StdEncoding.EncodeToString(raw)
		nextCursor = &enc
	}

	limit := filter.Limit
	if limit == 0 {
		limit = 50
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": dtos,
		"pagination": map[string]any{
			"next_cursor": nextCursor,
			"limit":       limit,
		},
	})
}

func (h *deliveryHandlers) getDelivery(w http.ResponseWriter, r *http.Request) {
	deliveryID, err := uuid.Parse(chi.URLParam(r, "deliveryId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid delivery id"))
		return
	}
	requester := mustUserID(r)
	d, err := h.svc.GetDelivery(r.Context(), deliveryID, requester)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toDeliveryDTO(d)})
}
