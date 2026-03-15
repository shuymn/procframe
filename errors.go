package procframe

import "fmt"

// Code represents a canonical error code used across all transports.
type Code string

const (
	CodeInvalidArgument  Code = "invalid_argument"
	CodeNotFound         Code = "not_found"
	CodeInternal         Code = "internal"
	CodeUnauthenticated  Code = "unauthenticated"
	CodeUnavailable      Code = "unavailable"
	CodeAlreadyExists    Code = "already_exists"
	CodePermissionDenied Code = "permission_denied"
	CodeConflict         Code = "conflict"
)

// Error is a structured error carrying a canonical code, human-readable
// message, optional cause, and a retryable hint for callers.
type Error struct {
	Code      Code
	Message   string
	Cause     error
	Retryable bool
}

// Error returns a string in the form "<code>: <message>".
// The Cause is intentionally omitted; use [errors.Unwrap] to traverse
// the chain.
func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying Cause so that [errors.Is] and
// [errors.As] work through the error chain.
func (e *Error) Unwrap() error {
	return e.Cause
}
