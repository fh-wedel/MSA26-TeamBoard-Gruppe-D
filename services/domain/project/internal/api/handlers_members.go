package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/teamboard/services/domain/project/internal/domain"
)

func (h *Handlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid project_id")
		return
	}

	members, err := h.svc.ListMembers(r.Context(), projectID, mustUserID(r))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	resp := make([]memberResponse, len(members))
	for i, m := range members {
		resp[i] = mapMemberResponse(m)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) AddMember(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid project_id")
		return
	}

	var req addMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	var targetUserID uuid.UUID
	if req.UserID != "" {
		if targetUserID, err = uuid.Parse(req.UserID); err != nil {
			writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid user_id")
			return
		}
	}

	member, err := h.svc.AddMember(r.Context(), projectID, mustUserID(r), targetUserID, req.Email, domain.Role(req.Role))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, mapMemberResponse(member))
}

func (h *Handlers) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid project_id")
		return
	}
	targetUserID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid user_id")
		return
	}

	var req updateMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	member, err := h.svc.UpdateMemberRole(r.Context(), projectID, mustUserID(r), targetUserID, domain.Role(req.Role))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapMemberResponse(member))
}

func (h *Handlers) RemoveMember(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid project_id")
		return
	}
	targetUserID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid user_id")
		return
	}

	if err := h.svc.RemoveMember(r.Context(), projectID, mustUserID(r), targetUserID); err != nil {
		writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
