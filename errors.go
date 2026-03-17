package procframe

import (
	"errors"
	"fmt"
)

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

// Status is the transport-facing error metadata produced at boundaries.
type Status struct {
	Code      Code
	Message   string
	Retryable bool
}

// ErrorMapper maps an error to a transport-facing [Status].
// It returns false when the error should remain unclassified.
type ErrorMapper func(error) (*Status, bool)

// StatusError is the default structured error wrapper provided by procframe.
type StatusError struct {
	status *Status
	cause  error
}

// NewError creates a [StatusError] with the given code and message.
func NewError(code Code, msg string) *StatusError {
	return &StatusError{
		status: &Status{
			Code:    code,
			Message: msg,
		},
	}
}

// Errorf creates a [StatusError] with a formatted message.
func Errorf(code Code, format string, args ...any) *StatusError {
	return NewError(code, fmt.Sprintf(format, args...))
}

// WrapError creates a [StatusError] that wraps a cause error.
func WrapError(code Code, msg string, cause error) *StatusError {
	err := NewError(code, msg)
	err.cause = cause
	return err
}

// StatusOf extracts a [Status] from an error chain.
// It returns false when the chain does not contain a [StatusError].
func StatusOf(err error) (*Status, bool) {
	var statusErr *StatusError
	if errors.As(err, &statusErr) {
		return statusErr.Status(), true
	}
	return nil, false
}

// CodeOf extracts the [Code] from an error chain. It returns the code
// and true if the chain contains a [StatusError], or ("", false) otherwise.
func CodeOf(err error) (Code, bool) {
	status, ok := StatusOf(err)
	if ok {
		return status.Code, true
	}
	return "", false
}

// Code returns the canonical error code.
func (e *StatusError) Code() Code { return e.status.Code }

// Message returns the human-readable error message.
func (e *StatusError) Message() string { return e.status.Message }

// IsRetryable reports whether the caller may retry the operation.
func (e *StatusError) IsRetryable() bool { return e.status.Retryable }

// Status returns the transport-facing status carried by the error.
func (e *StatusError) Status() *Status { return e.status }

// WithRetryable returns a copy of e with the retryable flag set to true.
func (e *StatusError) WithRetryable() *StatusError {
	next := *e
	status := *e.status
	status.Retryable = true
	next.status = &status
	return &next
}

// Error returns a string in the form "<code>: <message>".
// The cause is intentionally omitted; use [errors.Unwrap] to traverse
// the chain.
func (e *StatusError) Error() string {
	return fmt.Sprintf("%s: %s", e.status.Code, e.status.Message)
}

// Unwrap returns the underlying cause so that [errors.Is] and
// [errors.As] work through the error chain.
func (e *StatusError) Unwrap() error {
	return e.cause
}
