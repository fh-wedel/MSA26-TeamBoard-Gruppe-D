package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) AddAttachment(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid task_id")
		return
	}

	var req addAttachmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	att, err := h.svc.AddAttachment(r.Context(), taskID, mustUserID(r), req.DocumentID)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, mapAttachment(att))
}

func (h *Handlers) ListAttachments(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid task_id")
		return
	}

	attachments, err := h.svc.ListAttachments(r.Context(), taskID, mustUserID(r))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	resp := make([]attachmentResponse, len(attachments))
	for i, a := range attachments {
		resp[i] = mapAttachment(a)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) RemoveAttachment(w http.ResponseWriter, r *http.Request) {
	attachmentID, err := uuid.Parse(chi.URLParam(r, "attachmentID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid attachment_id")
		return
	}

	if err := h.svc.RemoveAttachment(r.Context(), attachmentID, mustUserID(r)); err != nil {
		writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
