// Package domain contains the core business logic of the auth service.
package domain

import "fmt"

// Error is a typed domain error with a machine-readable code.
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

func (e *Error) Unwrap() error { return e.Cause }

// GetCode satisfies the httputil.DomainError interface.
func (e *Error) GetCode() string { return e.Code }

// Sentinel domain errors.
var (
	ErrEmailTaken         = &Error{Code: "email_taken", Message: "email already registered"}
	ErrInvalidCredentials = &Error{Code: "invalid_credentials", Message: "invalid email or password"}
	ErrPasswordTooWeak    = &Error{Code: "password_too_weak", Message: "password does not meet policy"}
	ErrTokenInvalid       = &Error{Code: "token_invalid", Message: "token is invalid or expired"}
	ErrTokenRevoked       = &Error{Code: "token_revoked", Message: "token has been revoked"}
	ErrUserNotFound       = &Error{Code: "user_not_found", Message: "user not found"}
	ErrRateLimited        = &Error{Code: "rate_limited", Message: "too many attempts, try again later"}
	ErrSigningKeyNotFound = &Error{Code: "signing_key_not_found", Message: "no active signing key available"}
)
