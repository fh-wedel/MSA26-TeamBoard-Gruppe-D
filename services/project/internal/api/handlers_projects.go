package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/teamboard/services/project/internal/domain"
)

func (h *Handlers) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	ownerID := mustUserID(r)
	proj, err := h.svc.CreateProject(r.Context(), ownerID, req.Name, req.Description)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, mapProjectResponse(proj))
}

func (h *Handlers) GetProject(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid project_id")
		return
	}

	proj, err := h.svc.GetProject(r.Context(), projectID, mustUserID(r))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapProjectResponse(proj))
}

func (h *Handlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.svc.ListProjectsForUser(r.Context(), mustUserID(r))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	resp := make([]projectResponse, len(projects))
	for i, p := range projects {
		resp[i] = mapProjectResponse(p)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) UpdateProject(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid project_id")
		return
	}

	var req updateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	proj, err := h.svc.UpdateProject(r.Context(), projectID, mustUserID(r), domain.ProjectPatch{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapProjectResponse(proj))
}

func (h *Handlers) DeleteProject(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid project_id")
		return
	}

	if err := h.svc.DeleteProject(r.Context(), projectID, mustUserID(r)); err != nil {
		writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
