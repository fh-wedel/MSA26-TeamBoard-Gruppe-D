package domain

// Error is a typed domain error (matches the convention used across services).
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

var (
	ErrNotFound         = &Error{Code: "board_type_not_found", Message: "board type not found"}
	ErrAlreadyExists    = &Error{Code: "board_type_exists", Message: "board type already exists"}
	ErrValidation       = &Error{Code: "validation_failed", Message: "validation failed"}
	ErrBuiltInImmutable = &Error{Code: "builtin_immutable", Message: "built-in board types cannot be modified or deleted"}
)
