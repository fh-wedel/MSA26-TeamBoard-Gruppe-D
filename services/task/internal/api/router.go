package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/teamboard/services/task/internal/domain"
	"github.com/teamboard/shared/go/authmiddleware"
)

// Handlers holds the domain service used by all HTTP handlers.
type Handlers struct {
	svc domain.TaskService
}

// NewRouter wires all routes and returns the root handler.
func NewRouter(svc domain.TaskService, jwks authmiddleware.JWKSSource, jwtIssuer, jwtAudience string) http.Handler {
	h := &Handlers{svc: svc}
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/healthz/live", h.Liveness)
	r.Get("/healthz/ready", h.Readiness)

	// Public API documentation (OpenAPI spec + Swagger UI), no auth.
	mountDocs(r)

	r.Group(func(r chi.Router) {
		r.Use(newJWTMiddleware(jwks, jwtIssuer, jwtAudience))

		// Tasks via board
		r.Route("/api/v1/boards/{boardID}/tasks", func(r chi.Router) {
			r.Post("/", h.CreateTask)
			r.Get("/", h.ListTasks)
		})

		// Tasks direct
		r.Route("/api/v1/tasks/{taskID}", func(r chi.Router) {
			r.Get("/", h.GetTask)
			r.Patch("/", h.UpdateTask)
			r.Delete("/", h.DeleteTask)
			r.Post("/move", h.MoveTask)
			r.Post("/assign", h.AssignTask)

			// Comments
			r.Route("/comments", func(r chi.Router) {
				r.Get("/", h.ListComments)
				r.Post("/", h.CreateComment)
				r.Patch("/{commentID}", h.UpdateComment)
				r.Delete("/{commentID}", h.DeleteComment)
			})

			// Attachments
			r.Route("/attachments", func(r chi.Router) {
				r.Get("/", h.ListAttachments)
				r.Post("/", h.AddAttachment)
				r.Delete("/{attachmentID}", h.RemoveAttachment)
			})

			// History
			r.Get("/history", h.GetTaskHistory)
		})
	})

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

func writeJSONPaginated(w http.ResponseWriter, status int, data any, pagination paginationResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "pagination": pagination})
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
	slog.ErrorContext(r.Context(), "unmapped domain error", "error", err, "trace_id", middleware.GetReqID(r.Context()))
	writeProblem(w, r, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
}

var domainStatusMap = map[string]int{
	"validation_failed":       http.StatusBadRequest,
	"task_not_found":          http.StatusNotFound,
	"comment_not_found":       http.StatusNotFound,
	"attachment_not_found":    http.StatusNotFound,
	"board_unknown":           http.StatusBadRequest,
	"column_not_in_board":     http.StatusBadRequest,
	"assignee_not_member":     http.StatusBadRequest,
	"permission_denied":       http.StatusForbidden,
	"not_comment_author":      http.StatusForbidden,
	"document_not_in_project": http.StatusBadRequest,
	"document_unreachable":    http.StatusBadGateway,
	"invalid_position":        http.StatusConflict,
}
