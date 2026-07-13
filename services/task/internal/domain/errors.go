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
	ErrTaskNotFound         = &Error{Code: "task_not_found", Message: "task not found"}
	ErrCommentNotFound      = &Error{Code: "comment_not_found", Message: "comment not found"}
	ErrAttachmentNotFound   = &Error{Code: "attachment_not_found", Message: "attachment not found"}
	ErrBoardUnknown         = &Error{Code: "board_unknown", Message: "board not known to this service yet"}
	ErrColumnNotInBoard     = &Error{Code: "column_not_in_board", Message: "column does not belong to this board"}
	ErrAssigneeNotMember    = &Error{Code: "assignee_not_member", Message: "assignee is not a project member"}
	ErrPermissionDenied     = &Error{Code: "permission_denied", Message: "permission denied"}
	ErrNotCommentAuthor     = &Error{Code: "not_comment_author", Message: "only the comment author may edit this comment"}
	ErrDocumentNotInProject = &Error{Code: "document_not_in_project", Message: "document does not belong to this project"}
	ErrDocumentUnreachable  = &Error{Code: "document_unreachable", Message: "document service unavailable"}
	ErrInvalidPosition      = &Error{Code: "invalid_position", Message: "invalid task position"}
	ErrValidation           = &Error{Code: "validation_failed", Message: "validation failed"}
)
