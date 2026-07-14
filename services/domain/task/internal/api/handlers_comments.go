package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) CreateComment(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid task_id")
		return
	}

	var req createCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	comment, err := h.svc.CreateComment(r.Context(), taskID, mustUserID(r), req.Body)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, mapComment(comment))
}

func (h *Handlers) ListComments(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid task_id")
		return
	}

	comments, err := h.svc.ListComments(r.Context(), taskID, mustUserID(r))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	resp := make([]commentResponse, len(comments))
	for i, c := range comments {
		resp[i] = mapComment(c)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) UpdateComment(w http.ResponseWriter, r *http.Request) {
	commentID, err := uuid.Parse(chi.URLParam(r, "commentID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid comment_id")
		return
	}

	var req updateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	comment, err := h.svc.UpdateComment(r.Context(), commentID, mustUserID(r), req.Body)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapComment(comment))
}

func (h *Handlers) DeleteComment(w http.ResponseWriter, r *http.Request) {
	commentID, err := uuid.Parse(chi.URLParam(r, "commentID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid comment_id")
		return
	}

	if err := h.svc.DeleteComment(r.Context(), commentID, mustUserID(r)); err != nil {
		writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
