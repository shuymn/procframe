package ws

import "github.com/shuymn/procframe"

// toErrorFrame converts a handler error to an outbound error frame.
//
// The mapping chain mirrors the Connect transport:
//  1. StatusError in error chain -> its code/message/retryable
//  2. ErrorMapper (if configured) -> the returned status
//  3. Fallback -> CodeInternal
func toErrorFrame(id string, err error, mapper procframe.ErrorMapper) outboundFrame {
	status, ok := procframe.StatusOf(err)
	if !ok && mapper != nil {
		status, ok = mapper(err)
	}
	if !ok {
		status = &procframe.Status{
			Code:    procframe.CodeInternal,
			Message: err.Error(),
		}
	}
	return outboundFrame{
		Type: frameTypeError,
		ID:   id,
		Error: &errorDetail{
			Code:      string(status.Code),
			Message:   status.Message,
			Retryable: status.Retryable,
		},
	}
}
