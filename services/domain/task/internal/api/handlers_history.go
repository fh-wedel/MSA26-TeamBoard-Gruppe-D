package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) GetTaskHistory(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid task_id")
		return
	}

	entries, err := h.svc.GetTaskHistory(r.Context(), taskID, mustUserID(r))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	resp := make([]historyEntryResponse, len(entries))
	for i, e := range entries {
		resp[i] = mapHistoryEntry(e)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) Liveness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *Handlers) Readiness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
