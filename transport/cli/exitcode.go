package cli

import "github.com/shuymn/procframe"

// ExitCode returns the process exit code corresponding to the given
// procframe error code. Unknown codes map to 1.
func ExitCode(code procframe.Code) int {
	switch code {
	case procframe.CodeInternal:
		return 1
	case procframe.CodeInvalidArgument:
		return 2
	case procframe.CodeNotFound:
		return 3
	case procframe.CodeUnauthenticated:
		return 4
	case procframe.CodePermissionDenied:
		return 5
	case procframe.CodeConflict:
		return 6
	case procframe.CodeAlreadyExists:
		return 7
	case procframe.CodeUnavailable:
		return 8
	default:
		return 1
	}
}
