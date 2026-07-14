package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/teamboard/services/domain/notification/internal/domain"
	"github.com/teamboard/services/domain/notification/internal/push"
	"github.com/teamboard/services/domain/notification/internal/ws"
	"github.com/teamboard/shared/go/authmiddleware"
)

func NewRouter(svc domain.NotificationService, pushSvc *push.Service, hub *ws.Hub, jwks authmiddleware.JWKSSource, jwtIssuer, jwtAudience string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	verifier := newTokenVerifier(jwks, jwtIssuer, jwtAudience)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	// Public API documentation (OpenAPI spec + Swagger UI), no auth.
	mountDocs(r)

	// WebSocket endpoint — token in query string (validated inside the handler).
	r.Get("/ws", handleWebSocket(pushSvc, hub, verifier))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(verifier.middleware)

		r.Get("/notifications", handleListNotifications(svc))
		r.Get("/notifications/unread-count", handleGetUnreadCount(svc))
		r.Post("/notifications/read-all", handleMarkAllRead(svc))
		r.Post("/notifications/{id}/read", handleMarkRead(svc))
		r.Delete("/notifications/{id}", handleDeleteNotification(svc))
	})

	return r
}
