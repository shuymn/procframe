package ws

import (
	"context"
	"encoding/json"
	"io"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/shuymn/procframe"
)

// serverStream adapts the WS write channel to [procframe.ServerStream].
// Each Send writes an outbound message frame.
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
	s.writeFn(outboundFrame{Type: frameTypeMessage, ID: s.id, Payload: data})
	return nil
}

// wsClientStream reads from a session's recvCh and unmarshals JSON to
// *Request[Req]. EOF is signaled when recvCh is closed.
type wsClientStream[Req any] struct {
	getCtx func() context.Context
	recvCh <-chan json.RawMessage
}

func (s *wsClientStream[Req]) Context() context.Context { return s.getCtx() }

func (s *wsClientStream[Req]) Receive() (*procframe.Request[Req], error) {
	return receiveRequest[Req](s.recvCh)
}

// wsBidiStream reads from a session's recvCh and writes via writeFn.
type wsBidiStream[Req, Res any] struct {
	getCtx  func() context.Context
	recvCh  <-chan json.RawMessage
	id      string
	writeFn func(outboundFrame)
}

func (s *wsBidiStream[Req, Res]) Context() context.Context { return s.getCtx() }

func (s *wsBidiStream[Req, Res]) Receive() (*procframe.Request[Req], error) {
	return receiveRequest[Req](s.recvCh)
}

func (s *wsBidiStream[Req, Res]) Send(resp *procframe.Response[Res]) error {
	data, err := marshalProto(resp.Msg)
	if err != nil {
		return err
	}
	s.writeFn(outboundFrame{Type: frameTypeMessage, ID: s.id, Payload: data})
	return nil
}

// receiveRequest reads a single JSON payload from recvCh, unmarshals it,
// and wraps it in a Request. Returns io.EOF when recvCh is closed.
func receiveRequest[Req any](recvCh <-chan json.RawMessage) (*procframe.Request[Req], error) {
	payload, ok := <-recvCh
	if !ok {
		return nil, io.EOF
	}
	var req Req
	msg, ok := any(&req).(proto.Message)
	if !ok {
		return nil, procframe.Errorf(procframe.CodeInternal, "request type %T does not implement proto.Message", req)
	}
	if err := protojson.Unmarshal(payload, msg); err != nil {
		return nil, procframe.NewError(procframe.CodeInvalidArgument, err.Error())
	}
	return &procframe.Request[Req]{Msg: &req}, nil
}
