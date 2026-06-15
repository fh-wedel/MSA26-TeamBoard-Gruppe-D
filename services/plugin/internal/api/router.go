package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamboard/services/plugin/internal/domain"
)

func NewRouter(svc domain.WebhookService, pool *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	wh := &webhookHandlers{svc: svc}
	dl := &deliveryHandlers{svc: svc}
	he := &healthHandlers{pool: pool}

	r.Get("/health/live", he.live)
	r.Get("/health/ready", he.ready)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(jwtMiddleware)

		// Project-scoped webhook listing / creation
		r.Route("/projects/{projectId}/webhooks", func(r chi.Router) {
			r.Get("/", wh.listWebhooks)
			r.Post("/", wh.createWebhook)
		})

		// Webhook operations
		r.Route("/webhooks/{webhookId}", func(r chi.Router) {
			r.Get("/", wh.getWebhook)
			r.Patch("/", wh.updateWebhook)
			r.Delete("/", wh.deleteWebhook)
			r.Post("/rotate-secret", wh.rotateSecret)
			r.Post("/enable", wh.enableWebhook)
			r.Post("/disable", wh.disableWebhook)
			r.Post("/test", wh.triggerTest)

			// Deliveries
			r.Get("/deliveries", dl.listDeliveries)
			r.Get("/deliveries/{deliveryId}", dl.getDelivery)
		})
	})

	return r
}
