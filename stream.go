package procframe

import "context"

// ServerStream is the server-side interface for sending a sequence of
// responses back to the caller. Concrete implementations are provided
// by each transport (e.g. transport/cli).
type ServerStream[Res any] interface {
	Context() context.Context
	Send(*Response[Res]) error
}
