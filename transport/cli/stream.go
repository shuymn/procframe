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

// StreamReader implements [procframe.ClientStream] for CLI transports.
// The read function is provided by generated code, which performs
// NDJSON deserialization from stdin.
type StreamReader[Req any] struct {
	getCtx func() context.Context
	read   func() (*procframe.Request[Req], error)
}

// NewStreamReader constructs a [StreamReader] with the given context
// and read callback.
func NewStreamReader[Req any](ctx context.Context, read func() (*procframe.Request[Req], error)) *StreamReader[Req] {
	return &StreamReader[Req]{getCtx: func() context.Context { return ctx }, read: read}
}

// Context returns the context associated with this stream.
func (s *StreamReader[Req]) Context() context.Context {
	return s.getCtx()
}

// Receive reads a single request from the stream via the read callback.
func (s *StreamReader[Req]) Receive() (*procframe.Request[Req], error) {
	return s.read()
}

// BidiReadWriter implements [procframe.BidiStream] for CLI transports.
// The read and write functions are provided by generated code.
type BidiReadWriter[Req, Res any] struct {
	getCtx func() context.Context
	read   func() (*procframe.Request[Req], error)
	write  func(*procframe.Response[Res]) error
}

// NewBidiReadWriter constructs a [BidiReadWriter] with the given context,
// read, and write callbacks.
func NewBidiReadWriter[Req, Res any](
	ctx context.Context,
	read func() (*procframe.Request[Req], error),
	write func(*procframe.Response[Res]) error,
) *BidiReadWriter[Req, Res] {
	return &BidiReadWriter[Req, Res]{getCtx: func() context.Context { return ctx }, read: read, write: write}
}

// Context returns the context associated with this stream.
func (s *BidiReadWriter[Req, Res]) Context() context.Context {
	return s.getCtx()
}

// Receive reads a single request from the stream.
func (s *BidiReadWriter[Req, Res]) Receive() (*procframe.Request[Req], error) {
	return s.read()
}

// Send writes a single response to the stream.
func (s *BidiReadWriter[Req, Res]) Send(resp *procframe.Response[Res]) error {
	return s.write(resp)
}
