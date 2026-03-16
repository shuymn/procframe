package procframe

import "context"

// ServerStream is the server-side interface for sending a sequence of
// responses back to the caller. Concrete implementations are provided
// by each transport (e.g. transport/cli).
type ServerStream[Res any] interface {
	Context() context.Context
	Send(*Response[Res]) error
}

// ClientStream is the server-side interface for receiving a sequence of
// requests from the caller. Concrete implementations are provided
// by each transport.
type ClientStream[Req any] interface {
	Context() context.Context
	Receive() (*Request[Req], error)
}

// BidiStream is the server-side interface for bidirectional streaming.
// It supports both receiving requests and sending responses concurrently.
// Concrete implementations are provided by each transport.
type BidiStream[Req, Res any] interface {
	Context() context.Context
	Receive() (*Request[Req], error)
	Send(*Response[Res]) error
}
