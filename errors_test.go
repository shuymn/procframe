package procframe_test

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/shuymn/procframe"
)

// Compile-time assertions.
var (
	_ procframe.Error = (*procframe.StatusError)(nil) //nolint:errcheck // compile-time interface assertion
	_ error           = (*procframe.StatusError)(nil) //nolint:errcheck // compile-time interface assertion
)

func TestStatusError_Error(t *testing.T) {
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

			msg := tc.want[len(string(tc.code))+2:]
			e := procframe.NewError(tc.code, msg)

			if got := e.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStatusError_Accessors(t *testing.T) {
	t.Parallel()

	e := procframe.NewError(procframe.CodeNotFound, "gone")
	if e.Code() != procframe.CodeNotFound {
		t.Errorf("Code() = %q, want %q", e.Code(), procframe.CodeNotFound)
	}
	if e.Message() != "gone" {
		t.Errorf("Message() = %q, want %q", e.Message(), "gone")
	}
	if e.IsRetryable() {
		t.Error("IsRetryable() = true, want false")
	}
}

func TestStatusError_Unwrap(t *testing.T) {
	t.Parallel()

	t.Run("with cause", func(t *testing.T) {
		t.Parallel()

		cause := io.EOF
		e := procframe.WrapError(procframe.CodeInternal, "wrap", cause)

		if got := e.Unwrap(); !errors.Is(got, cause) {
			t.Errorf("Unwrap() = %v, want %v", got, cause)
		}
	})

	t.Run("nil cause", func(t *testing.T) {
		t.Parallel()

		e := procframe.NewError(procframe.CodeInternal, "no cause")

		if got := e.Unwrap(); got != nil {
			t.Errorf("Unwrap() = %v, want nil", got)
		}
	})
}

func TestStatusError_WithRetryable(t *testing.T) {
	t.Parallel()

	t.Run("true", func(t *testing.T) {
		t.Parallel()

		e := procframe.NewError(procframe.CodeUnavailable, "busy").WithRetryable()

		var target procframe.Error
		if !errors.As(e, &target) {
			t.Fatal("errors.As failed")
		}

		if !target.IsRetryable() {
			t.Error("IsRetryable() = false, want true")
		}
	})

	t.Run("default false", func(t *testing.T) {
		t.Parallel()

		e := procframe.NewError(procframe.CodeNotFound, "gone")

		var target procframe.Error
		if !errors.As(e, &target) {
			t.Fatal("errors.As failed")
		}

		if target.IsRetryable() {
			t.Error("IsRetryable() = true, want false")
		}
	})
}

func TestStatusError_Errorf(t *testing.T) {
	t.Parallel()

	e := procframe.Errorf(procframe.CodeInternal, "count: %d", 42)
	if got := e.Error(); got != "internal: count: 42" {
		t.Errorf("Error() = %q, want %q", got, "internal: count: 42")
	}
}

func TestStatusError_ErrorsIs(t *testing.T) {
	t.Parallel()

	cause := io.EOF
	e := procframe.WrapError(procframe.CodeInternal, "wrap", cause)

	if !errors.Is(e, io.EOF) {
		t.Error("errors.Is(e, io.EOF) = false, want true")
	}
}

func TestStatusError_ErrorsAs_Interface(t *testing.T) {
	t.Parallel()

	inner := procframe.NewError(procframe.CodeNotFound, "inner")
	outer := procframe.WrapError(procframe.CodeInternal, "outer", inner)

	var target procframe.Error
	if !errors.As(outer, &target) {
		t.Fatal("errors.As failed")
	}

	if target.Code() != procframe.CodeInternal {
		t.Errorf("Code() = %q, want %q", target.Code(), procframe.CodeInternal)
	}

	// Traverse to the inner error through the chain.
	unwrapper, ok := target.(interface{ Unwrap() error })
	if !ok {
		t.Fatal("target does not implement Unwrap")
	}
	var inner2 procframe.Error
	if !errors.As(unwrapper.Unwrap(), &inner2) {
		t.Fatal("errors.As on unwrapped failed")
	}

	if inner2.Code() != procframe.CodeNotFound {
		t.Errorf("inner Code() = %q, want %q", inner2.Code(), procframe.CodeNotFound)
	}
}

func TestCodeOf(t *testing.T) {
	t.Parallel()

	t.Run("StatusError", func(t *testing.T) {
		t.Parallel()

		e := procframe.NewError(procframe.CodeNotFound, "gone")
		code, ok := procframe.CodeOf(e)
		if !ok {
			t.Fatal("CodeOf returned false")
		}
		if code != procframe.CodeNotFound {
			t.Errorf("CodeOf = %q, want %q", code, procframe.CodeNotFound)
		}
	})

	t.Run("wrapped StatusError", func(t *testing.T) {
		t.Parallel()

		inner := procframe.NewError(procframe.CodeInternal, "inner")
		outer := fmt.Errorf("wrap: %w", inner)
		code, ok := procframe.CodeOf(outer)
		if !ok {
			t.Fatal("CodeOf returned false")
		}
		if code != procframe.CodeInternal {
			t.Errorf("CodeOf = %q, want %q", code, procframe.CodeInternal)
		}
	})

	t.Run("non-procframe error", func(t *testing.T) {
		t.Parallel()

		_, ok := procframe.CodeOf(io.EOF)
		if ok {
			t.Error("CodeOf returned true for non-procframe error")
		}
	})

	t.Run("nil error", func(t *testing.T) {
		t.Parallel()

		_, ok := procframe.CodeOf(nil)
		if ok {
			t.Error("CodeOf returned true for nil error")
		}
	})
}

// customError is a user-defined type that implements procframe.Error.
type customError struct {
	code procframe.Code
	msg  string
}

func (e *customError) Error() string        { return fmt.Sprintf("%s: %s", e.code, e.msg) }
func (e *customError) Code() procframe.Code { return e.code }
func (e *customError) Message() string      { return e.msg }
func (e *customError) IsRetryable() bool    { return false }

func TestErrorsAs_UserDefinedError(t *testing.T) {
	t.Parallel()

	custom := &customError{code: procframe.CodeConflict, msg: "version mismatch"}

	var target procframe.Error
	if !errors.As(custom, &target) {
		t.Fatal("errors.As failed for user-defined Error")
	}
	if target.Code() != procframe.CodeConflict {
		t.Errorf("Code() = %q, want %q", target.Code(), procframe.CodeConflict)
	}

	// CodeOf should also work with user-defined types.
	code, ok := procframe.CodeOf(custom)
	if !ok {
		t.Fatal("CodeOf returned false for user-defined Error")
	}
	if code != procframe.CodeConflict {
		t.Errorf("CodeOf = %q, want %q", code, procframe.CodeConflict)
	}
}
