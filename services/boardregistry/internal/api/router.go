package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teamboard/services/boardregistry/internal/domain"
	"github.com/teamboard/shared/go/authmiddleware"
	"github.com/teamboard/shared/go/servicetoken"
)

// NewRouter wires the HTTP routes for the board-registry service.
func NewRouter(svc domain.BoardTypeService, pool *pgxpool.Pool, jwks authmiddleware.JWKSSource, jwtIssuer, jwtAudience string, stVerifier servicetoken.Verifier, patIntrospector authmiddleware.TokenIntrospector) http.Handler {
	h := &handlers{svc: svc}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/healthz/live", liveness)
	r.Get("/healthz/ready", readiness(pool))

	// Public API documentation (OpenAPI spec + Swagger UI), no auth.
	mountDocs(r)

	// Public catalog (read) + developer registration (write), authenticated via JWT.
	// NOTE: write endpoints are currently open to any authenticated user. Restricting
	// registration to an admin/publisher role is tracked as follow-up work.
	r.Group(func(r chi.Router) {
		r.Use(newJWTMiddleware(jwks, jwtIssuer, jwtAudience, patIntrospector))
		r.Route("/api/v1/board-types", func(r chi.Router) {
			r.Get("/", h.list)
			r.Post("/", h.register)
			r.Route("/{type}", func(r chi.Router) {
				r.Get("/", h.get)
				r.Patch("/", h.update)
				r.Delete("/", h.remove)
			})
		})
	})

	// Internal catalog consumed by the project service (short-lived service token).
	r.Group(func(r chi.Router) {
		r.Use(servicetoken.RequireServiceToken(stVerifier))
		r.Get("/api/v1/internal/board-types", h.list)
		r.Get("/api/v1/internal/board-types/{type}", h.get)
	})

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":    "about:blank",
		"title":   http.StatusText(status),
		"status":  status,
		"detail":  message,
		"code":    code,
		"traceId": middleware.GetReqID(r.Context()),
	})
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var de *domain.Error
	if errors.As(err, &de) {
		status, ok := domainStatusMap[de.Code]
		if !ok {
			status = http.StatusInternalServerError
		}
		writeProblem(w, r, status, de.Code, de.Message)
		return
	}
	writeProblem(w, r, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
}

var domainStatusMap = map[string]int{
	"validation_failed":    http.StatusBadRequest,
	"board_type_not_found": http.StatusNotFound,
	"board_type_exists":    http.StatusConflict,
	"builtin_immutable":    http.StatusForbidden,
}

func liveness(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func readiness(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2_000_000_000)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			writeProblem(w, r, http.StatusServiceUnavailable, "not_ready", "database unavailable")
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}
}
