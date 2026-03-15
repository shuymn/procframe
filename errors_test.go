package procframe_test

import (
	"errors"
	"io"
	"testing"

	"github.com/shuymn/procframe"
)

// Compile-time verification that *Error implements the error interface.
var _ error = (*procframe.Error)(nil) //nolint:errcheck // compile-time interface assertion, not a function call

func TestError_Error(t *testing.T) {
	t.Parallel()

	codes := []struct {
		code procframe.Code
		want string
	}{
		{procframe.CodeInvalidArgument, "invalid_argument: bad input"},
		{procframe.CodeNotFound, "not_found: missing"},
		{procframe.CodeInternal, "internal: oops"},
		{procframe.CodeUnauthenticated, "unauthenticated: no token"},
		{procframe.CodeUnavailable, "unavailable: try later"},
		{procframe.CodeAlreadyExists, "already_exists: duplicate"},
		{procframe.CodePermissionDenied, "permission_denied: forbidden"},
		{procframe.CodeConflict, "conflict: version mismatch"},
	}

	for _, tc := range codes {
		t.Run(string(tc.code), func(t *testing.T) {
			t.Parallel()

			// Extract message from expected string after ": "
			msg := tc.want[len(string(tc.code))+2:]
			e := &procframe.Error{Code: tc.code, Message: msg}

			if got := e.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	t.Parallel()

	t.Run("with cause", func(t *testing.T) {
		t.Parallel()

		cause := io.EOF
		e := &procframe.Error{Code: procframe.CodeInternal, Message: "wrap", Cause: cause}

		if got := e.Unwrap(); !errors.Is(got, cause) {
			t.Errorf("Unwrap() = %v, want %v", got, cause)
		}
	})

	t.Run("nil cause", func(t *testing.T) {
		t.Parallel()

		e := &procframe.Error{Code: procframe.CodeInternal, Message: "no cause"}

		if got := e.Unwrap(); got != nil {
			t.Errorf("Unwrap() = %v, want nil", got)
		}
	})
}

func TestError_Retryable(t *testing.T) {
	t.Parallel()

	t.Run("true", func(t *testing.T) {
		t.Parallel()

		e := &procframe.Error{Code: procframe.CodeUnavailable, Message: "busy", Retryable: true}

		var target *procframe.Error
		if !errors.As(e, &target) {
			t.Fatal("errors.As failed")
		}

		if !target.Retryable {
			t.Error("Retryable = false, want true")
		}
	})

	t.Run("default false", func(t *testing.T) {
		t.Parallel()

		e := &procframe.Error{Code: procframe.CodeNotFound, Message: "gone"}

		var target *procframe.Error
		if !errors.As(e, &target) {
			t.Fatal("errors.As failed")
		}

		if target.Retryable {
			t.Error("Retryable = true, want false")
		}
	})
}

func TestError_ErrorsIs(t *testing.T) {
	t.Parallel()

	cause := io.EOF
	e := &procframe.Error{Code: procframe.CodeInternal, Message: "wrap", Cause: cause}

	if !errors.Is(e, io.EOF) {
		t.Error("errors.Is(e, io.EOF) = false, want true")
	}
}

func TestError_ErrorsAs(t *testing.T) {
	t.Parallel()

	inner := &procframe.Error{Code: procframe.CodeNotFound, Message: "inner"}
	outer := &procframe.Error{Code: procframe.CodeInternal, Message: "outer", Cause: inner}

	var target *procframe.Error
	if !errors.As(outer, &target) {
		t.Fatal("errors.As failed")
	}

	if target.Code != procframe.CodeInternal {
		t.Errorf("Code = %q, want %q", target.Code, procframe.CodeInternal)
	}

	// Traverse to the inner error through the chain.
	var inner2 *procframe.Error
	if !errors.As(target.Unwrap(), &inner2) {
		t.Fatal("errors.As on unwrapped failed")
	}

	if inner2.Code != procframe.CodeNotFound {
		t.Errorf("inner Code = %q, want %q", inner2.Code, procframe.CodeNotFound)
	}
}
