package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/shuymn/procframe"
)

// writeBufSize is the default buffer size for the per-connection write channel.
const writeBufSize = 64

// handler handles a single dispatched WS request.
type handler func(ctx context.Context, id string, payload json.RawMessage, writeFn func(outboundFrame))

// Server is a WebSocket RPC server that dispatches inbound frames to
// registered procedure handlers.
type Server struct {
	handlers map[string]handler
	opts     options
}

// NewServer creates a new WebSocket RPC server with the given options.
func NewServer(opts ...Option) *Server {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return &Server{
		handlers: make(map[string]handler),
		opts:     o,
	}
}

// HandleUnary registers a unary RPC handler for the given procedure.
func HandleUnary[Req, Res any](
	s *Server,
	procedure string,
	h func(context.Context, *procframe.Request[Req]) (*procframe.Response[Res], error),
) {
	s.handlers[procedure] = func(ctx context.Context, id string, payload json.RawMessage, writeFn func(outboundFrame)) {
		req, err := unmarshalRequest[Req](payload)
		if err != nil {
			writeFn(toErrorFrame(id, err, s.opts.errorMapper))
			return
		}
		resp, err := procframe.InvokeUnary(
			ctx,
			procframe.CallSpec{
				Procedure: procedure,
				Transport: procframe.TransportWS,
				Shape:     procframe.CallShapeUnary,
			},
			&procframe.Request[Req]{
				Msg:  req,
				Meta: procframe.Meta{Procedure: procedure, RequestID: id},
			},
			h,
			s.opts.interceptors...,
		)
		if err != nil {
			writeFn(toErrorFrame(id, err, s.opts.errorMapper))
			return
		}
		if resp == nil || resp.Msg == nil {
			writeFn(
				toErrorFrame(
					id,
					procframe.NewError(procframe.CodeInternal, "handler returned nil response"),
					s.opts.errorMapper,
				),
			)
			return
		}
		data, err := marshalProto(resp.Msg)
		if err != nil {
			writeFn(toErrorFrame(id, procframe.NewError(procframe.CodeInternal, err.Error()), s.opts.errorMapper))
			return
		}
		writeFn(outboundFrame{
			ID:      id,
			Payload: data,
			EOS:     true,
		})
	}
}

// HandleServerStream registers a server-streaming RPC handler for the given procedure.
func HandleServerStream[Req, Res any](
	s *Server,
	procedure string,
	h func(context.Context, *procframe.Request[Req], procframe.ServerStream[Res]) error,
) {
	s.handlers[procedure] = func(ctx context.Context, id string, payload json.RawMessage, writeFn func(outboundFrame)) {
		req, err := unmarshalRequest[Req](payload)
		if err != nil {
			writeFn(toErrorFrame(id, err, s.opts.errorMapper))
			return
		}
		stream := &serverStream[Res]{
			getCtx:  func() context.Context { return ctx },
			id:      id,
			writeFn: writeFn,
		}
		err = procframe.InvokeServerStream(
			ctx,
			procframe.CallSpec{
				Procedure: procedure,
				Transport: procframe.TransportWS,
				Shape:     procframe.CallShapeServerStream,
			},
			&procframe.Request[Req]{
				Msg:  req,
				Meta: procframe.Meta{Procedure: procedure, RequestID: id},
			},
			stream,
			h,
			s.opts.interceptors...,
		)
		if err != nil {
			writeFn(toErrorFrame(id, err, s.opts.errorMapper))
			return
		}
		// Final EOS frame signals stream completion.
		writeFn(outboundFrame{ID: id, EOS: true})
	}
}

// ServeHTTP upgrades the HTTP request to a WebSocket connection and runs
// the read-dispatch-write loop.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow() //nolint:errcheck // best-effort cleanup; error not actionable in defer

	s.serve(r.Context(), conn)
}

// serve runs the core read-dispatch-write loop on a WS connection.
func (s *Server) serve(baseCtx context.Context, conn *websocket.Conn) {
	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	writeCh := make(chan []byte, writeBufSize)

	var writeWG sync.WaitGroup
	writeWG.Go(func() {
		s.runWriter(ctx, conn, writeCh)
	})

	writeFn := func(out outboundFrame) {
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return
		}
		select {
		case writeCh <- data:
		case <-ctx.Done():
		}
	}

	sem := make(chan struct{}, s.opts.maxInflight)
	var handleWG sync.WaitGroup

	for {
		_, data, rErr := conn.Read(ctx)
		if rErr != nil {
			break
		}
		var frame inboundFrame
		if uErr := json.Unmarshal(data, &frame); uErr != nil {
			continue
		}
		s.dispatch(ctx, frame, sem, writeFn, &handleWG)
	}

	cancel() // Cancel handler contexts on disconnect.
	handleWG.Wait()
	close(writeCh)
	writeWG.Wait()
}

// runWriter drains writeCh and sends each message to the WS connection.
func (s *Server) runWriter(ctx context.Context, conn *websocket.Conn, writeCh <-chan []byte) {
	for data := range writeCh {
		if wErr := conn.Write(ctx, websocket.MessageText, data); wErr != nil {
			//nolint:revive // drain channel to unblock sender goroutines
			for range writeCh {
			}
			return
		}
	}
}

// dispatch routes an inbound frame to the registered handler, applying
// semaphore-based inflight control.
func (s *Server) dispatch(
	ctx context.Context,
	frame inboundFrame,
	sem chan struct{},
	writeFn func(outboundFrame),
	handleWG *sync.WaitGroup,
) {
	h, ok := s.handlers[frame.Procedure]
	if !ok {
		writeFn(outboundFrame{
			ID: frame.ID,
			Error: &errorDetail{
				Code:    string(procframe.CodeNotFound),
				Message: "unknown procedure: " + frame.Procedure,
			},
			EOS: true,
		})
		return
	}

	select {
	case sem <- struct{}{}:
	default:
		writeFn(outboundFrame{
			ID: frame.ID,
			Error: &errorDetail{
				Code:      string(procframe.CodeUnavailable),
				Message:   "max inflight exceeded",
				Retryable: true,
			},
			EOS: true,
		})
		return
	}

	handleWG.Go(func() {
		defer func() { <-sem }()
		h(ctx, frame.ID, frame.Payload, writeFn)
	})
}

// unmarshalRequest deserializes a JSON payload into a proto request message.
func unmarshalRequest[Req any](payload json.RawMessage) (*Req, error) {
	var req Req
	msg, ok := any(&req).(proto.Message)
	if !ok {
		return nil, fmt.Errorf("request type %T does not implement proto.Message", req)
	}
	if err := protojson.Unmarshal(payload, msg); err != nil {
		return nil, procframe.NewError(procframe.CodeInvalidArgument, err.Error())
	}
	return &req, nil
}

// marshalProto serializes a response message to JSON.
func marshalProto[T any](msg *T) (json.RawMessage, error) {
	pm, ok := any(msg).(proto.Message)
	if !ok {
		return nil, fmt.Errorf("response type %T does not implement proto.Message", msg)
	}
	data, err := protojson.Marshal(pm)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
