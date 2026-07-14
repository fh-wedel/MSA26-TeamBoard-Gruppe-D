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
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error   { return e.Cause }
func (e *Error) GetCode() string { return e.Code }

var (
	ErrNotificationNotFound = &Error{Code: "notification_not_found", Message: "notification not found"}
	ErrPermissionDenied     = &Error{Code: "permission_denied", Message: "permission denied"}
	ErrInvalidChannel       = &Error{Code: "invalid_channel", Message: "invalid channel format"}
	ErrSubscriptionDenied   = &Error{Code: "subscription_denied", Message: "subscription denied"}
	ErrUnauthorized         = &Error{Code: "unauthorized", Message: "unauthorized"}
)
