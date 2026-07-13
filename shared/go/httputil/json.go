package httputil

import (
	"encoding/json"
	"net/http"
)

type dataResponse struct {
	Data any `json:"data"`
}

type listResponse struct {
	Data       any         `json:"data"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Pagination holds cursor-based pagination metadata.
type Pagination struct {
	NextCursor *string `json:"next_cursor"`
	Limit      int     `json:"limit"`
}

// WriteJSON writes data wrapped in {"data": ...} with the given status.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(dataResponse{Data: data})
}

// WriteList writes a list response with optional pagination.
func WriteList(w http.ResponseWriter, status int, items any, pagination *Pagination) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(listResponse{Data: items, Pagination: pagination})
}
