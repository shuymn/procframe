package ws

import (
	"context"

	"github.com/shuymn/procframe"
)

// serverStream adapts the WS write channel to [procframe.ServerStream].
// Each Send writes an outbound frame with eos=false.
type serverStream[Res any] struct {
	getCtx  func() context.Context
	id      string
	writeFn func(outboundFrame)
}

func (s *serverStream[Res]) Context() context.Context { return s.getCtx() }

func (s *serverStream[Res]) Send(resp *procframe.Response[Res]) error {
	data, err := marshalProto(resp.Msg)
	if err != nil {
		return err
	}
	s.writeFn(outboundFrame{ID: s.id, Payload: data, EOS: false})
	return nil
}
