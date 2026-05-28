package abyss

import "fmt"

var (
	ErrNotFound         = newError("not_found", "resource not found")
	ErrUnauthorized     = newError("unauthorized", "unauthorized")
	ErrForbidden        = newError("forbidden", "forbidden")
	ErrPermissionDenied = ErrForbidden
	ErrInvalidInput     = newError("invalid_input", "invalid input")
	ErrConflict         = newError("conflict", "resource conflict")
	ErrInternal         = newError("internal", "internal server error")
	ErrEmptyField       = newError("empty_field", "field cannot be empty")
)

// Error is the canonical application error type.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Cause   error  `json:"-"`
}

func newError(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// WrapError wraps a base error with a cause and optional message override.
func WrapError(base *Error, cause error, message string) *Error {
	if base == nil {
		base = ErrInternal
	}
	m := base.Message
	if message != "" {
		m = message
	}
	return &Error{Code: base.Code, Message: m, Cause: cause}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
