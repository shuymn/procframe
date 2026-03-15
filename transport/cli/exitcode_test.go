package cli_test

import (
	"testing"

	"github.com/shuymn/procframe"
	"github.com/shuymn/procframe/transport/cli"
)

func TestExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code procframe.Code
		want int
	}{
		{procframe.CodeInternal, 1},
		{procframe.CodeInvalidArgument, 2},
		{procframe.CodeNotFound, 3},
		{procframe.CodeUnauthenticated, 4},
		{procframe.CodePermissionDenied, 5},
		{procframe.CodeConflict, 6},
		{procframe.CodeAlreadyExists, 7},
		{procframe.CodeUnavailable, 8},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			t.Parallel()
			if got := cli.ExitCode(tt.code); got != tt.want {
				t.Fatalf("ExitCode(%q) = %d, want %d", tt.code, got, tt.want)
			}
		})
	}

	t.Run("unknown code defaults to 1", func(t *testing.T) {
		t.Parallel()
		if got := cli.ExitCode("unknown_code"); got != 1 {
			t.Fatalf("ExitCode(%q) = %d, want 1", "unknown_code", got)
		}
	})
}
