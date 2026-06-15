package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/teamboard/services/notification/internal/domain"
	"github.com/teamboard/services/notification/internal/push"
	"github.com/teamboard/services/notification/internal/ws"
)

func NewRouter(svc domain.NotificationService, pushSvc *push.Service, hub *ws.Hub) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	// WebSocket endpoint — token in query string, no JWT middleware here.
	r.Get("/ws", handleWebSocket(pushSvc, hub))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(jwtMiddleware)

		r.Get("/notifications", handleListNotifications(svc))
		r.Get("/notifications/unread-count", handleGetUnreadCount(svc))
		r.Post("/notifications/read-all", handleMarkAllRead(svc))
		r.Post("/notifications/{id}/read", handleMarkRead(svc))
		r.Delete("/notifications/{id}", handleDeleteNotification(svc))
	})

	return r
}
