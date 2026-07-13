// Package httputil provides RFC 7807 problem details, JSON helpers, and cursor pagination.
package httputil

import (
	"encoding/json"
	"net/http"

	"go.opentelemetry.io/otel/trace"
)

// Problem represents an RFC 7807 Problem Details response.
type Problem struct {
	Type     string       `json:"type"`
	Title    string       `json:"title"`
	Status   int          `json:"status"`
	Detail   string       `json:"detail,omitempty"`
	Instance string       `json:"instance,omitempty"`
	TraceID  string       `json:"trace_id,omitempty"`
	Errors   []FieldError `json:"errors,omitempty"`
}

// FieldError describes a single validation failure.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// WriteProblem writes an RFC 7807 problem details response.
func WriteProblem(w http.ResponseWriter, r *http.Request, status int, detail string) {
	title := http.StatusText(status)
	WriteProblemFull(w, r, Problem{
		Type:    "https://teamboard.example/errors/" + statusSlug(status),
		Title:   title,
		Status:  status,
		Detail:  detail,
		TraceID: traceIDFromContext(r),
	})
}

// WriteProblemFull writes a fully populated RFC 7807 response.
func WriteProblemFull(w http.ResponseWriter, r *http.Request, p Problem) {
	if p.TraceID == "" {
		p.TraceID = traceIDFromContext(r)
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

func traceIDFromContext(r *http.Request) string {
	if span := trace.SpanFromContext(r.Context()); span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

func statusSlug(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case http.StatusInternalServerError:
		return "internal_error"
	default:
		return "error"
	}
}
