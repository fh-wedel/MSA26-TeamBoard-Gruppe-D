package httputil

import (
	"errors"
	"net/http"
	"strings"
)

// DomainError is the interface domain errors must satisfy for automatic HTTP mapping.
type DomainError interface {
	error
	GetCode() string
}

// WriteError maps a domain error to an RFC 7807 response via MapErrorToHTTPStatus.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var de DomainError
	if !errors.As(err, &de) {
		WriteProblem(w, r, http.StatusInternalServerError, "internal error")
		return
	}

	status := MapErrorToHTTPStatus(err)
	WriteProblemFull(w, r, Problem{
		Type:    "https://teamboard.example/errors/" + de.GetCode(),
		Title:   humanizeCode(de.GetCode()),
		Status:  status,
		Detail:  de.Error(),
		TraceID: traceIDFromContext(r),
	})
}

// MapErrorToHTTPStatus returns the HTTP status for a domain error.
func MapErrorToHTTPStatus(err error) int {
	var de DomainError
	if !errors.As(err, &de) {
		return http.StatusInternalServerError
	}
	code := de.GetCode()
	switch {
	case strings.HasSuffix(code, "_not_found"):
		return http.StatusNotFound
	case code == "permission_denied":
		return http.StatusForbidden
	case code == "validation_failed", code == "password_too_weak",
		code == "invalid_url", strings.HasPrefix(code, "invalid_"):
		return http.StatusBadRequest
	case strings.HasPrefix(code, "already_"), code == "conflict",
		code == "last_owner_protected", code == "email_taken":
		return http.StatusConflict
	case code == "rate_limited":
		return http.StatusTooManyRequests
	case code == "unauthorized", code == "token_invalid",
		code == "token_revoked", code == "invalid_credentials":
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

func humanizeCode(code string) string {
	replacer := strings.NewReplacer("_", " ")
	s := replacer.Replace(code)
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
