package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/teamboard/services/project/internal/domain"
)

type Handlers struct {
	svc domain.ProjectService
}

func NewRouter(svc domain.ProjectService, serviceTokenSecret string) http.Handler {
	h := &Handlers{svc: svc}
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// Health
	r.Get("/healthz/live", h.Liveness)
	r.Get("/healthz/ready", h.Readiness)

	// Auth-protected routes
	r.Group(func(r chi.Router) {
		r.Use(jwtMiddleware)

		r.Route("/api/v1/projects", func(r chi.Router) {
			r.Post("/", h.CreateProject)
			r.Get("/", h.ListProjects)

			r.Route("/{projectID}", func(r chi.Router) {
				r.Get("/", h.GetProject)
				r.Patch("/", h.UpdateProject)
				r.Delete("/", h.DeleteProject)

				r.Route("/members", func(r chi.Router) {
					r.Get("/", h.ListMembers)
					r.Post("/", h.AddMember)
					r.Patch("/{userID}/role", h.UpdateMemberRole)
					r.Delete("/{userID}", h.RemoveMember)
				})

				r.Route("/boards", func(r chi.Router) {
					r.Get("/", h.ListBoards)
					r.Post("/", h.CreateBoard)
				})

				r.Route("/invitations", func(r chi.Router) {
					r.Get("/", h.ListInvitations)
					r.Post("/", h.CreateInvitation)
				})
			})
		})

		// Board-type catalog is served by the board registry service (routed at the
		// gateway). The project service consumes its internal API via boardtypeclient.

		// Invitation accept/decline (token-based, invitee must be logged in)
		r.Route("/api/v1/invitations/{token}", func(r chi.Router) {
			r.Get("/", h.GetInvitation)
			r.Post("/accept", h.AcceptInvitation)
			r.Post("/decline", h.DeclineInvitation)
		})

		r.Route("/api/v1/boards", func(r chi.Router) {
			r.Route("/{boardID}", func(r chi.Router) {
				r.Get("/", h.GetBoard)
				r.Patch("/", h.UpdateBoard)
				r.Delete("/", h.DeleteBoard)

				r.Route("/columns", func(r chi.Router) {
					r.Get("/", h.ListColumns)
					r.Post("/", h.CreateColumn)
				})
			})
		})
	})

	// Internal service-to-service routes
	r.Group(func(r chi.Router) {
		r.Use(serviceTokenMiddleware(serviceTokenSecret))
		r.Get("/api/v1/internal/projects/{projectID}/permissions/{userID}", h.GetPermissions)
	})

	return r
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

// writeProblem writes an RFC 7807 problem details response.
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

// writeDomainError maps domain errors to HTTP responses.
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
	"validation_failed":   http.StatusBadRequest,
	"project_not_found":   http.StatusNotFound,
	"board_not_found":     http.StatusNotFound,
	"member_not_found":    http.StatusNotFound,
	"unknown_user":        http.StatusNotFound,
	"already_member":      http.StatusConflict,
	"invalid_role":        http.StatusBadRequest,
	"invalid_board_type":  http.StatusBadRequest,
	"board_type_registry_unavailable": http.StatusServiceUnavailable,
	"last_owner":          http.StatusConflict,
	"permission_denied":   http.StatusForbidden,
}
