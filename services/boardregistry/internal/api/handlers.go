package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/teamboard/services/boardregistry/internal/domain"
)

type handlers struct {
	svc domain.BoardTypeService
}

func (h *handlers) list(w http.ResponseWriter, r *http.Request) {
	defs, err := h.svc.List(r.Context())
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	resp := make([]boardTypeResponse, len(defs))
	for i, d := range defs {
		resp[i] = toResponse(d)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *handlers) get(w http.ResponseWriter, r *http.Request) {
	def, err := h.svc.Get(r.Context(), chi.URLParam(r, "type"))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(def))
}

func (h *handlers) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}
	userID := mustUserID(r)
	def, err := h.svc.Register(r.Context(), domain.RegisterInput{
		Type:           req.Type,
		DisplayName:    req.DisplayName,
		Icon:           req.Icon,
		DefaultColumns: toColumnDefs(req.DefaultColumns),
		DefaultConfig:  req.DefaultConfig,
		ConfigSchema:   req.ConfigSchema,
		CreatedBy:      &userID,
	})
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toResponse(def))
}

func (h *handlers) update(w http.ResponseWriter, r *http.Request) {
	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}
	patch := domain.UpdatePatch{
		DisplayName:   req.DisplayName,
		Icon:          req.Icon,
		DefaultConfig: req.DefaultConfig,
		ConfigSchema:  req.ConfigSchema,
	}
	if req.DefaultColumns != nil {
		cols := toColumnDefs(*req.DefaultColumns)
		patch.DefaultColumns = &cols
	}
	def, err := h.svc.Update(r.Context(), chi.URLParam(r, "type"), patch)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(def))
}

func (h *handlers) remove(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), chi.URLParam(r, "type")); err != nil {
		writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
