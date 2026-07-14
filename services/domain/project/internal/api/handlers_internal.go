package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) GetPermissions(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid project_id")
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid user_id")
		return
	}

	ps, err := h.svc.GetPermissions(r.Context(), projectID, userID)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapPermissionResponse(ps))
}
