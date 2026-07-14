package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/teamboard/services/domain/project/internal/domain"
)

func (h *Handlers) CreateBoard(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid project_id")
		return
	}

	var req createBoardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	cols := make([]domain.BoardColumnInput, len(req.Columns))
	for i, c := range req.Columns {
		cols[i] = domain.BoardColumnInput{Name: c.Name, Position: c.Position, WIPLimit: c.WIPLimit, Status: c.Status}
	}

	board, err := h.svc.CreateBoard(r.Context(), projectID, mustUserID(r), domain.BoardInput{
		Name:    req.Name,
		Type:    domain.BoardType(req.Type),
		Config:  req.Config,
		Columns: cols,
	})
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, mapBoardResponse(board))
}

func (h *Handlers) GetBoard(w http.ResponseWriter, r *http.Request) {
	boardID, err := uuid.Parse(chi.URLParam(r, "boardID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid board_id")
		return
	}

	board, err := h.svc.GetBoard(r.Context(), boardID, mustUserID(r))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapBoardResponse(board))
}

func (h *Handlers) ListBoards(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid project_id")
		return
	}

	boards, err := h.svc.ListBoards(r.Context(), projectID, mustUserID(r))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	resp := make([]boardResponse, len(boards))
	for i, b := range boards {
		resp[i] = mapBoardResponse(b)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) UpdateBoard(w http.ResponseWriter, r *http.Request) {
	boardID, err := uuid.Parse(chi.URLParam(r, "boardID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid board_id")
		return
	}

	var req updateBoardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	board, err := h.svc.UpdateBoard(r.Context(), boardID, mustUserID(r), domain.BoardPatch{
		Name:   req.Name,
		Config: req.Config,
	})
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapBoardResponse(board))
}

func (h *Handlers) DeleteBoard(w http.ResponseWriter, r *http.Request) {
	boardID, err := uuid.Parse(chi.URLParam(r, "boardID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid board_id")
		return
	}

	if err := h.svc.DeleteBoard(r.Context(), boardID, mustUserID(r)); err != nil {
		writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ListColumns(w http.ResponseWriter, r *http.Request) {
	boardID, err := uuid.Parse(chi.URLParam(r, "boardID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid board_id")
		return
	}

	cols, err := h.svc.ListColumns(r.Context(), boardID, mustUserID(r))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	resp := make([]columnResponse, len(cols))
	for i, c := range cols {
		resp[i] = mapColumnResponse(c)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) CreateColumn(w http.ResponseWriter, r *http.Request) {
	boardID, err := uuid.Parse(chi.URLParam(r, "boardID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid board_id")
		return
	}

	var req createColumnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	col, err := h.svc.CreateColumn(r.Context(), boardID, mustUserID(r), domain.BoardColumnInput{
		Name:     req.Name,
		Position: req.Position,
		WIPLimit: req.WIPLimit,
		Status:   req.Status,
	})
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, mapColumnResponse(*col))
}
