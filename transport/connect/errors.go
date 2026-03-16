package connect

import (
	"errors"

	connectrpc "connectrpc.com/connect"

	"github.com/shuymn/procframe"
)

// toConnectError maps a procframe error to a Connect error.
//
// The mapping chain mirrors the CLI transport:
//  1. StatusError in error chain → map its code
//  2. ErrorMapper (if configured) → map the returned status
//  3. Fallback → connect.CodeInternal
func toConnectError(err error, mapper procframe.ErrorMapper) error {
	if status, ok := procframe.StatusOf(err); ok {
		return connectrpc.NewError(mapCode(status.Code), errors.New(status.Message))
	}
	if mapper != nil {
		if status, ok := mapper(err); ok {
			return connectrpc.NewError(mapCode(status.Code), errors.New(status.Message))
		}
	}
	return connectrpc.NewError(connectrpc.CodeInternal, err)
}

// mapCode maps a procframe error code to a Connect error code.
func mapCode(code procframe.Code) connectrpc.Code {
	switch code {
	case procframe.CodeInvalidArgument:
		return connectrpc.CodeInvalidArgument
	case procframe.CodeNotFound:
		return connectrpc.CodeNotFound
	case procframe.CodeInternal:
		return connectrpc.CodeInternal
	case procframe.CodeUnauthenticated:
		return connectrpc.CodeUnauthenticated
	case procframe.CodeUnavailable:
		return connectrpc.CodeUnavailable
	case procframe.CodeAlreadyExists:
		return connectrpc.CodeAlreadyExists
	case procframe.CodePermissionDenied:
		return connectrpc.CodePermissionDenied
	case procframe.CodeConflict:
		return connectrpc.CodeAborted
	default:
		return connectrpc.CodeInternal
	}
}
