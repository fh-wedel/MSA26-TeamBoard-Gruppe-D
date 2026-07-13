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

func (e *Error) Unwrap() error    { return e.Cause }
func (e *Error) GetCode() string  { return e.Code }

var (
	ErrProjectNotFound    = &Error{Code: "project_not_found",    Message: "project not found"}
	ErrBoardNotFound      = &Error{Code: "board_not_found",      Message: "board not found"}
	ErrMemberNotFound     = &Error{Code: "member_not_found",     Message: "member not found"}
	ErrAlreadyMember      = &Error{Code: "already_member",       Message: "user is already a member"}
	ErrPermissionDenied   = &Error{Code: "permission_denied",    Message: "permission denied"}
	ErrLastOwner          = &Error{Code: "last_owner_protected",  Message: "cannot remove or demote the last owner"}
	ErrUnknownUser        = &Error{Code: "unknown_user",         Message: "user not found in this service"}
	ErrInvalidRole        = &Error{Code: "invalid_role",         Message: "invalid role"}
	ErrInvalidBoardType   = &Error{Code: "invalid_board_type",   Message: "invalid board type"}
	ErrBoardTypeRegistryUnavailable = &Error{Code: "board_type_registry_unavailable", Message: "board type registry is unavailable"}
	ErrValidation         = &Error{Code: "validation_failed",    Message: "validation failed"}
	ErrInvitationNotFound = &Error{Code: "invitation_not_found", Message: "invitation not found"}
	ErrInvitationExpired  = &Error{Code: "invitation_expired",   Message: "invitation has expired"}
	ErrInvitationNotPending = &Error{Code: "invitation_not_pending", Message: "invitation is no longer pending"}
	ErrInvitationMismatch = &Error{Code: "invitation_mismatch",  Message: "invitation was not issued to this user"}
	ErrAlreadyInvited     = &Error{Code: "already_invited",      Message: "user already has a pending invitation for this project"}
)
