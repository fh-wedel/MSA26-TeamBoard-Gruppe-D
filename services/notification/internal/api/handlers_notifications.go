package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/teamboard/services/notification/internal/domain"
)

func handleListNotifications(svc domain.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := mustUserID(r)
		q := r.URL.Query()

		filter := domain.ListFilter{}
		if q.Get("unread_only") == "true" {
			filter.UnreadOnly = true
		}
		if t := q.Get("type"); t != "" {
			nt := domain.NotificationType(t)
			filter.Type = &nt
		}
		filter.Limit = 50
		if l := q.Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil {
				filter.Limit = n
			}
		}
		if c := q.Get("cursor"); c != "" {
			filter.Cursor = &c
		}

		ns, nextCursor, err := svc.ListNotifications(r.Context(), userID, filter)
		if err != nil {
			writeError(w, r, err)
			return
		}

		dtos := make([]notificationDTO, len(ns))
		for i, n := range ns {
			dtos[i] = mapNotification(n)
		}
		var pagination *paginationDTO
		if nc := encodeCursor(nextCursor); nc != nil {
			pagination = &paginationDTO{NextCursor: nc}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(notificationListDTO{Data: dtos, Pagination: pagination})
	}
}

func handleGetUnreadCount(svc domain.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := mustUserID(r)
		count, err := svc.GetUnreadCount(r.Context(), userID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"unread_count": count})
	}
}

func handleMarkRead(svc domain.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := mustUserID(r)
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
			return
		}
		if err := svc.MarkRead(r.Context(), id, userID); err != nil {
			writeError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleMarkAllRead(svc domain.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := mustUserID(r)
		if err := svc.MarkAllRead(r.Context(), userID); err != nil {
			writeError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleDeleteNotification(svc domain.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := mustUserID(r)
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
			return
		}
		if err := svc.DeleteNotification(r.Context(), id, userID); err != nil {
			writeError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ── Shared response helpers ───────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var de *domain.Error
	if errors.As(err, &de) {
		status := domainErrStatus(de.Code)
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":     "about:blank",
			"title":    de.Message,
			"status":   status,
			"code":     de.Code,
			"trace_id": middleware.GetReqID(r.Context()),
		})
		return
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":     "about:blank",
		"title":    "internal server error",
		"status":   http.StatusInternalServerError,
		"trace_id": middleware.GetReqID(r.Context()),
	})
}

func domainErrStatus(code string) int {
	switch code {
	case domain.ErrNotificationNotFound.Code:
		return http.StatusNotFound
	case domain.ErrPermissionDenied.Code:
		return http.StatusForbidden
	case domain.ErrUnauthorized.Code:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}
