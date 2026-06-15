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
	ErrDocumentNotFound       = &Error{Code: "document_not_found", Message: "document not found"}
	ErrVersionNotFound        = &Error{Code: "version_not_found", Message: "version not found"}
	ErrDocumentNotActive      = &Error{Code: "document_not_active", Message: "document is not active"}
	ErrVersionNotPending      = &Error{Code: "version_not_pending", Message: "version is not in pending state"}
	ErrVersionNotUploaded     = &Error{Code: "version_not_uploaded", Message: "version has not been uploaded yet"}
	ErrPermissionDenied       = &Error{Code: "permission_denied", Message: "permission denied"}
	ErrFileTooLarge           = &Error{Code: "file_too_large", Message: "file exceeds maximum allowed size"}
	ErrUnsupportedContentType = &Error{Code: "unsupported_content_type", Message: "content type is not allowed"}
	ErrUploadSizeMismatch     = &Error{Code: "upload_size_mismatch", Message: "uploaded file size does not match the declared size"}
	ErrUploadNotFound         = &Error{Code: "upload_not_found_in_storage", Message: "uploaded file was not found in storage"}
	ErrProjectUnknown         = &Error{Code: "project_unknown", Message: "project is not known to this service yet"}
	ErrValidation             = &Error{Code: "validation_failed", Message: "validation failed"}
)
