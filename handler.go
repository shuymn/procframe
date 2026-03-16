package procframe

import "context"

// UnaryHandler handles a single request and returns a single response.
type UnaryHandler[Req, Res any] interface {
	Handle(context.Context, *Request[Req]) (*Response[Res], error)
}

// ClientStreamHandler handles a stream of requests and returns a single response.
type ClientStreamHandler[Req, Res any] interface {
	Handle(context.Context, ClientStream[Req]) (*Response[Res], error)
}

// ServerStreamHandler handles a single request and writes zero or more
// responses to the provided [ServerStream].
type ServerStreamHandler[Req, Res any] interface {
	HandleStream(context.Context, *Request[Req], ServerStream[Res]) error
}

// BidiStreamHandler handles a bidirectional stream of requests and responses.
type BidiStreamHandler[Req, Res any] interface {
	HandleBidi(context.Context, BidiStream[Req, Res]) error
}
