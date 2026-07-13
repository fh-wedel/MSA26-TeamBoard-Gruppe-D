package domain

import "fmt"

type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return e.Code
}

func (e *Error) Unwrap() error { return e.Cause }
func (e *Error) GetCode() string { return e.Code }

var (
	ErrWebhookNotFound     = &Error{Code: "webhook_not_found", Message: "webhook not found"}
	ErrDeliveryNotFound    = &Error{Code: "delivery_not_found", Message: "delivery not found"}
	ErrPermissionDenied    = &Error{Code: "permission_denied", Message: "permission denied"}
	ErrInvalidURL          = &Error{Code: "invalid_url", Message: "invalid webhook URL"}
	ErrPrivateURLForbidden = &Error{Code: "private_url_forbidden", Message: "private IP addresses are not allowed"}
	ErrInvalidEventFilter  = &Error{Code: "invalid_event_filter", Message: "invalid event filter pattern"}
	ErrProjectUnknown      = &Error{Code: "project_unknown", Message: "project not found or not accessible"}
	ErrCircuitOpen         = &Error{Code: "circuit_open", Message: "circuit breaker is open"}
	ErrValidation          = &Error{Code: "validation_failed", Message: "validation failed"}
	ErrNoPendingDelivery   = &Error{Code: "no_pending_delivery", Message: "no pending delivery"}
)
