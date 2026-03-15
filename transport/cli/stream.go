package cli

import (
	"context"

	"github.com/shuymn/procframe"
)

// StreamWriter implements [procframe.ServerStream] for CLI transports.
// The write function is provided by generated code, which performs
// format-specific marshalling (e.g. protojson).
type StreamWriter[Res any] struct {
	getCtx func() context.Context
	write  func(*procframe.Response[Res]) error
}

// NewStreamWriter constructs a [StreamWriter] with the given context
// and write callback.
func NewStreamWriter[Res any](ctx context.Context, write func(*procframe.Response[Res]) error) *StreamWriter[Res] {
	return &StreamWriter[Res]{getCtx: func() context.Context { return ctx }, write: write}
}

// Context returns the context associated with this stream.
func (s *StreamWriter[Res]) Context() context.Context {
	return s.getCtx()
}

// Send writes a single response to the stream via the write callback.
func (s *StreamWriter[Res]) Send(resp *procframe.Response[Res]) error {
	return s.write(resp)
}
