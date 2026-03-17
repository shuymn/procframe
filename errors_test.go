package procframe_test

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/shuymn/procframe"
)

// Compile-time assertions.
var (
	_ error = (*procframe.StatusError)(nil) //nolint:errcheck // compile-time interface assertion
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
	if got := e.Status(); got.Code != procframe.CodeNotFound || got.Message != "gone" || got.Retryable {
		t.Fatalf("Status() = %+v, want code/message/retryable=false", got)
	}
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

		status, ok := procframe.StatusOf(e)
		if !ok {
			t.Fatal("StatusOf returned false")
		}

		if !status.Retryable {
			t.Error("IsRetryable() = false, want true")
		}
	})

	t.Run("default false", func(t *testing.T) {
		t.Parallel()

		e := procframe.NewError(procframe.CodeNotFound, "gone")

		status, ok := procframe.StatusOf(e)
		if !ok {
			t.Fatal("StatusOf returned false")
		}

		if status.Retryable {
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

func TestStatusOf(t *testing.T) {
	t.Parallel()

	inner := procframe.NewError(procframe.CodeNotFound, "inner")
	outer := procframe.WrapError(procframe.CodeInternal, "outer", inner)

	status, ok := procframe.StatusOf(outer)
	if !ok {
		t.Fatal("StatusOf returned false")
	}

	if status.Code != procframe.CodeInternal {
		t.Errorf("Code = %q, want %q", status.Code, procframe.CodeInternal)
	}
	if status.Message != "outer" {
		t.Errorf("Message = %q, want %q", status.Message, "outer")
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

func TestStatusOf_NonStatusError(t *testing.T) {
	t.Parallel()

	_, ok := procframe.StatusOf(nonStatusError{})
	if ok {
		t.Fatal("StatusOf returned true for non-status error")
	}
}

func TestStatusOf_NilError(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("StatusOf(nil) panicked: %v", r)
		}
	}()

	_, ok := procframe.StatusOf(nil)
	if ok {
		t.Fatal("StatusOf(nil) should return false")
	}
}

func TestStatusError_ErrorNoInternalLeak(t *testing.T) {
	t.Parallel()

	err := procframe.NewError(procframe.CodeInternal, "user-visible message")
	msg := err.Error()

	if msg != "internal: user-visible message" {
		t.Fatalf("unexpected error format: %q", msg)
	}

	// Verify no stack trace or file paths.
	for _, leak := range []string{".go:", "goroutine", "runtime.", "panic"} {
		if strings.Contains(msg, leak) {
			t.Fatalf("error message leaks internal: %q found in %q", leak, msg)
		}
	}
}

func TestStatusError_UnknownCode(t *testing.T) {
	t.Parallel()

	err := procframe.NewError(procframe.Code("unknown_code"), "test")
	msg := err.Error()
	if msg != "unknown_code: test" {
		t.Fatalf("unexpected error format for unknown code: %q", msg)
	}
}

type nonStatusError struct{}

func (nonStatusError) Error() string { return "custom" }
