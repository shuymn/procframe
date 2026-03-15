package procframe

import "context"

// UnaryHandler handles a single request and returns a single response.
type UnaryHandler[Req, Res any] interface {
	Handle(context.Context, *Request[Req]) (*Response[Res], error)
}

// ServerStreamHandler handles a single request and writes zero or more
// responses to the provided [ServerStream].
type ServerStreamHandler[Req, Res any] interface {
	HandleStream(context.Context, *Request[Req], ServerStream[Res]) error
}
