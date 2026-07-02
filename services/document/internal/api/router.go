package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/teamboard/services/document/internal/domain"
	"github.com/teamboard/shared/go/authmiddleware"
	"github.com/teamboard/shared/go/servicetoken"
)

func NewRouter(svc domain.DocumentService, jwks authmiddleware.JWKSSource, jwtIssuer, jwtAudience string, stVerifier servicetoken.Verifier) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", handleLiveness)
	r.Get("/readyz", handleReadiness)

	// Public API documentation (OpenAPI spec + Swagger UI), no auth.
	mountDocs(r)

	// User-facing routes (RS256 user JWT).
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(newJWTMiddleware(jwks, jwtIssuer, jwtAudience))

		// Documents
		r.Post("/documents", handleInitiateUpload(svc))
		r.Get("/documents", handleListDocuments(svc))
		r.Get("/documents/{documentID}", handleGetDocument(svc))
		r.Delete("/documents/{documentID}", handleDeleteDocument(svc))
		r.Patch("/documents/{documentID}", handleRenameDocument(svc))

		// Upload lifecycle
		r.Post("/documents/{documentID}/versions", handleInitiateNewVersion(svc))
		r.Post("/documents/{documentID}/versions/{version}:confirm", handleConfirmUpload(svc))
		r.Get("/documents/{documentID}/versions", handleListVersions(svc))
		r.Post("/documents/{documentID}/versions/{version}:restore", handleRestoreVersion(svc))

		// Download
		r.Get("/documents/{documentID}/download", handleGetDownloadURL(svc))
	})

	// Internal service-to-service route (short-lived service token, aud "internal").
	r.Group(func(r chi.Router) {
		r.Use(servicetoken.RequireServiceToken(stVerifier))
		r.Get("/api/v1/internal/documents/{documentID}", handleGetDocumentInfo(svc))
	})

	return r
}

// ── Helpers ────────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var domErr *domain.Error
	if errors.As(err, &domErr) {
		status := domainErrStatus(domErr.Code)
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":   "about:blank",
			"title":  domErr.Message,
			"status": status,
			"code":   domErr.Code,
			"trace_id": middleware.GetReqID(r.Context()),
		})
		return
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  "internal server error",
		"status": http.StatusInternalServerError,
		"trace_id": middleware.GetReqID(r.Context()),
	})
}

func domainErrStatus(code string) int {
	switch code {
	case domain.ErrDocumentNotFound.Code, domain.ErrVersionNotFound.Code:
		return http.StatusNotFound
	case domain.ErrPermissionDenied.Code:
		return http.StatusForbidden
	case domain.ErrDocumentNotActive.Code,
		domain.ErrVersionNotPending.Code,
		domain.ErrVersionNotUploaded.Code,
		domain.ErrFileTooLarge.Code,
		domain.ErrUnsupportedContentType.Code,
		domain.ErrUploadSizeMismatch.Code,
		domain.ErrUploadNotFound.Code,
		domain.ErrValidation.Code:
		return http.StatusUnprocessableEntity
	case domain.ErrProjectUnknown.Code:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func handleLiveness(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func handleReadiness(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
