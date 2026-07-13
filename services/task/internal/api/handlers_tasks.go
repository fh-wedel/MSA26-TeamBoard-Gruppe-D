package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/teamboard/services/task/internal/domain"
)

func (h *Handlers) CreateTask(w http.ResponseWriter, r *http.Request) {
	boardID, err := uuid.Parse(chi.URLParam(r, "boardID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid board_id")
		return
	}

	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	input := domain.CreateTaskInput{
		BoardID:     boardID,
		ColumnID:    req.ColumnID,
		Title:       req.Title,
		Description: req.Description,
		Priority:    domain.Priority(req.Priority),
		AssigneeID:  req.AssigneeID,
		DueDate:     req.DueDate,
		StartDate:   req.StartDate,
		Labels:      req.Labels,
	}

	task, err := h.svc.CreateTask(r.Context(), mustUserID(r), input)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, mapTask(task))
}

func (h *Handlers) GetTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid task_id")
		return
	}

	task, err := h.svc.GetTask(r.Context(), taskID, mustUserID(r))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapTask(task))
}

func (h *Handlers) ListTasks(w http.ResponseWriter, r *http.Request) {
	boardID, err := uuid.Parse(chi.URLParam(r, "boardID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid board_id")
		return
	}

	filter := domain.TaskFilter{}
	if s := r.URL.Query().Get("status"); s != "" {
		st := domain.Status(s)
		filter.Status = &st
	}
	if a := r.URL.Query().Get("assignee_id"); a != "" {
		if id, err := uuid.Parse(a); err == nil {
			filter.AssigneeID = &id
		}
	}
	if c := r.URL.Query().Get("column_id"); c != "" {
		if id, err := uuid.Parse(c); err == nil {
			filter.ColumnID = &id
		}
	}
	if l := r.URL.Query().Get("label"); l != "" {
		filter.Label = &l
	}

	limit := 50
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if n, err := strconv.Atoi(lStr); err == nil && n > 0 {
			limit = n
		}
	}

	var cursor *string
	if c := r.URL.Query().Get("cursor"); c != "" {
		cursor = &c
	}

	tasks, nextCursor, err := h.svc.ListTasks(r.Context(), boardID, mustUserID(r), filter, domain.Pagination{
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	resp := make([]taskResponse, len(tasks))
	for i, t := range tasks {
		resp[i] = mapTask(t)
	}

	var nextCursorStr *string
	if nextCursor != nil {
		b, _ := json.Marshal(nextCursor)
		encoded := base64.StdEncoding.EncodeToString(b)
		nextCursorStr = &encoded
	}
	writeJSONPaginated(w, http.StatusOK, resp, paginationResponse{NextCursor: nextCursorStr, Limit: limit})
}

func (h *Handlers) UpdateTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid task_id")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "could not read body")
		return
	}

	patch, err := parseTaskPatch(body)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	task, err := h.svc.UpdateTask(r.Context(), taskID, mustUserID(r), patch)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapTask(task))
}

func (h *Handlers) MoveTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid task_id")
		return
	}

	var req moveTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	task, err := h.svc.MoveTask(r.Context(), taskID, mustUserID(r), req.ColumnID, req.BeforeID, req.AfterID)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapTask(task))
}

func (h *Handlers) AssignTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid task_id")
		return
	}

	var req assignTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	task, err := h.svc.AssignTask(r.Context(), taskID, mustUserID(r), req.AssigneeID)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapTask(task))
}

func (h *Handlers) DeleteTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid task_id")
		return
	}

	if err := h.svc.DeleteTask(r.Context(), taskID, mustUserID(r)); err != nil {
		writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseTaskPatch handles merge-patch semantics: detecting whether due_date was
// explicitly set (even to null) vs omitted.
func parseTaskPatch(body []byte) (domain.TaskPatch, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return domain.TaskPatch{}, err
	}

	var patch domain.TaskPatch

	if v, ok := raw["title"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return patch, err
		}
		patch.Title = &s
	}
	if v, ok := raw["description"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return patch, err
		}
		patch.Description = &s
	}
	if v, ok := raw["priority"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return patch, err
		}
		p := domain.Priority(s)
		patch.Priority = &p
	}
	if v, ok := raw["status"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return patch, err
		}
		st := domain.Status(s)
		patch.Status = &st
	}
	if v, ok := raw["due_date"]; ok {
		patch.DueDateSet = true
		if string(v) != "null" {
			var t time.Time
			if err := json.Unmarshal(v, &t); err != nil {
				return patch, err
			}
			patch.DueDate = &t
		}
	}
	if v, ok := raw["start_date"]; ok {
		patch.StartDateSet = true
		if string(v) != "null" {
			var t time.Time
			if err := json.Unmarshal(v, &t); err != nil {
				return patch, err
			}
			patch.StartDate = &t
		}
	}
	if v, ok := raw["labels"]; ok {
		var labels []string
		if err := json.Unmarshal(v, &labels); err != nil {
			return patch, err
		}
		patch.Labels = &labels
	}

	return patch, nil
}
