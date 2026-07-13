package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teamboard/services/auth/internal/domain"
	"github.com/teamboard/shared/go/servicetoken"
)

// Handlers bundles all HTTP handlers for the auth service.
type Handlers struct {
	svc  domain.AuthService
	repo domain.Repository
	db   *pgxpool.Pool
}

// NewHandlers creates a Handlers instance.
func NewHandlers(svc domain.AuthService, repo domain.Repository, pool *pgxpool.Pool) *Handlers {
	return &Handlers{svc: svc, repo: repo, db: pool}
}

// NewRouter wires the Chi router with all routes and middleware.
func NewRouter(svc domain.AuthService, repo domain.Repository, pool *pgxpool.Pool, issuer, audience string, stVerifier servicetoken.Verifier) http.Handler {
	h := NewHandlers(svc, repo, pool)
	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)

	// Health
	r.Get("/health/live", h.Liveness)
	r.Get("/health/ready", h.Readiness)

	// JWKS
	r.Get("/.well-known/jwks.json", h.JWKS)

	// Public API documentation (OpenAPI spec + Swagger UI), no auth.
	mountDocs(r)

	// Auth endpoints
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/register", h.Register)
		r.Post("/login", h.Login)
		r.Post("/refresh", h.Refresh)
		r.Post("/logout", h.Logout)
		r.Post("/password-reset/request", h.RequestPasswordReset)
		r.Post("/password-reset/confirm", h.ConfirmPasswordReset)

		// Protected: requires valid access token
		r.Group(func(r chi.Router) {
			r.Use(requireAuth(repo, issuer, audience))
			r.Get("/me", h.GetUser)

			r.Route("/tokens", func(r chi.Router) {
				r.Post("/", h.CreatePAT)
				r.Get("/", h.ListPATs)
				r.Delete("/{id}", h.RevokePAT)
			})
		})
	})

	// Internal service-to-service routes
	r.Group(func(r chi.Router) {
		r.Use(servicetoken.RequireServiceToken(stVerifier))
		r.Post("/api/v1/internal/tokens/introspect", h.IntrospectPAT)
	})

	return r
}

// ── Shared response helpers ──────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, errType, title string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   errType,
		"title":  title,
		"status": status,
	})
}

// domainErrorStatus maps known domain error codes to HTTP status codes.
var domainErrorStatus = map[string]int{
	"email_taken":          http.StatusConflict,
	"invalid_credentials":  http.StatusUnauthorized,
	"password_too_weak":    http.StatusUnprocessableEntity,
	"token_invalid":        http.StatusUnauthorized,
	"token_revoked":        http.StatusUnauthorized,
	"user_not_found":       http.StatusNotFound,
	"rate_limited":         http.StatusTooManyRequests,
	"signing_key_not_found": http.StatusInternalServerError,
}

type domainError interface {
	GetCode() string
	Error() string
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var de domainError
	if errors.As(err, &de) {
		status, ok := domainErrorStatus[de.GetCode()]
		if !ok {
			status = http.StatusInternalServerError
		}
		writeProblem(w, r, status, de.GetCode(), de.Error())
		return
	}
	writeProblem(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
}
