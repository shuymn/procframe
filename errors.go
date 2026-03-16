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

// Error is a structured error interface recognised by all transports.
// Any type implementing this interface is handled by the transport layer
// for exit-code mapping, JSON output, etc.
type Error interface {
	error
	Code() Code
	Message() string
	IsRetryable() bool
}

// StatusError is the default implementation of the [Error] interface
// provided by procframe.
type StatusError struct {
	code      Code
	msg       string
	cause     error
	retryable bool
}

// NewError creates a [StatusError] with the given code and message.
func NewError(code Code, msg string) *StatusError {
	return &StatusError{code: code, msg: msg}
}

// Errorf creates a [StatusError] with a formatted message.
func Errorf(code Code, format string, args ...any) *StatusError {
	return &StatusError{code: code, msg: fmt.Sprintf(format, args...)}
}

// WrapError creates a [StatusError] that wraps a cause error.
func WrapError(code Code, msg string, cause error) *StatusError {
	return &StatusError{code: code, msg: msg, cause: cause}
}

// CodeOf extracts the [Code] from an error chain. It returns the code
// and true if the chain contains an [Error], or ("", false) otherwise.
func CodeOf(err error) (Code, bool) {
	var pfErr Error
	if errors.As(err, &pfErr) {
		return pfErr.Code(), true
	}
	return "", false
}

// Code returns the canonical error code.
func (e *StatusError) Code() Code { return e.code }

// Message returns the human-readable error message.
func (e *StatusError) Message() string { return e.msg }

// IsRetryable reports whether the caller may retry the operation.
func (e *StatusError) IsRetryable() bool { return e.retryable }

// WithRetryable returns a copy of e with the retryable flag set to true.
func (e *StatusError) WithRetryable() *StatusError {
	return &StatusError{
		code:      e.code,
		msg:       e.msg,
		cause:     e.cause,
		retryable: true,
	}
}

// Error returns a string in the form "<code>: <message>".
// The cause is intentionally omitted; use [errors.Unwrap] to traverse
// the chain.
func (e *StatusError) Error() string {
	return fmt.Sprintf("%s: %s", e.code, e.msg)
}

// Unwrap returns the underlying cause so that [errors.Is] and
// [errors.As] work through the error chain.
func (e *StatusError) Unwrap() error {
	return e.cause
}
