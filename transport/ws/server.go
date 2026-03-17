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

// recvChBufSize is the buffer size for a session's receive channel.
const recvChBufSize = 64

// handlerEntry stores the shape and run function for a registered procedure.
type handlerEntry struct {
	shape procframe.CallShape
	run   func(ctx context.Context, id string, recvCh <-chan json.RawMessage, writeFn func(outboundFrame))
}

// sessionInfo tracks the state of an active session.
type sessionInfo struct {
	cancel context.CancelFunc
	recvCh chan json.RawMessage
	closed bool // true after handleClose; prevents message-after-close panic
}

// Server is a WebSocket RPC server that dispatches inbound frames to
// registered procedure handlers using the v2 session protocol.
type Server struct {
	handlers map[string]*handlerEntry
	opts     options
}

// NewServer creates a new WebSocket RPC server with the given options.
func NewServer(opts ...Option) *Server {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return &Server{
		handlers: make(map[string]*handlerEntry),
		opts:     o,
	}
}

// HandleUnary registers a unary RPC handler for the given procedure.
func HandleUnary[Req, Res any](
	s *Server,
	procedure string,
	h func(context.Context, *procframe.Request[Req]) (*procframe.Response[Res], error),
) {
	spec := procframe.CallSpec{
		Procedure: procedure,
		Transport: procframe.TransportWS,
		Shape:     procframe.CallShapeUnary,
	}
	s.handlers[procedure] = &handlerEntry{
		shape: procframe.CallShapeUnary,
		run: func(ctx context.Context, id string, recvCh <-chan json.RawMessage, writeFn func(outboundFrame)) {
			payload, ok := recvExactlyOne(recvCh)
			if !ok {
				errMsg := "session closed without request"
				writeFn(toErrorFrame(
					id,
					procframe.NewError(procframe.CodeInvalidArgument, errMsg),
					s.opts.errorMapper,
				))
				return
			}
			if hasDrained := drainAndVerifyEmpty(recvCh); !hasDrained {
				errMsg := "unary session received multiple messages"
				writeFn(toErrorFrame(
					id,
					procframe.NewError(procframe.CodeInvalidArgument, errMsg),
					s.opts.errorMapper,
				))
				return
			}

			req, err := unmarshalRequest[Req](payload)
			if err != nil {
				writeFn(toErrorFrame(id, err, s.opts.errorMapper))
				return
			}
			resp, err := procframe.InvokeUnary(ctx, spec,
				&procframe.Request[Req]{
					Msg:  req,
					Meta: procframe.Meta{Procedure: procedure, RequestID: id},
				}, h, s.opts.interceptors...)
			if err != nil {
				writeFn(toErrorFrame(id, err, s.opts.errorMapper))
				return
			}
			writeResponseAndClose(id, resp, writeFn, s.opts.errorMapper)
		},
	}
}

// HandleServerStream registers a server-streaming RPC handler for the given procedure.
func HandleServerStream[Req, Res any](
	s *Server,
	procedure string,
	h func(context.Context, *procframe.Request[Req], procframe.ServerStream[Res]) error,
) {
	spec := procframe.CallSpec{
		Procedure: procedure,
		Transport: procframe.TransportWS,
		Shape:     procframe.CallShapeServerStream,
	}
	s.handlers[procedure] = &handlerEntry{
		shape: procframe.CallShapeServerStream,
		run: func(ctx context.Context, id string, recvCh <-chan json.RawMessage, writeFn func(outboundFrame)) {
			payload, ok := recvExactlyOne(recvCh)
			if !ok {
				errMsg := "session closed without request"
				writeFn(toErrorFrame(
					id,
					procframe.NewError(procframe.CodeInvalidArgument, errMsg),
					s.opts.errorMapper,
				))
				return
			}
			if hasDrained := drainAndVerifyEmpty(recvCh); !hasDrained {
				errMsg := "server-stream session received multiple messages"
				writeFn(toErrorFrame(
					id,
					procframe.NewError(procframe.CodeInvalidArgument, errMsg),
					s.opts.errorMapper,
				))
				return
			}

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
			err = procframe.InvokeServerStream(ctx, spec,
				&procframe.Request[Req]{
					Msg:  req,
					Meta: procframe.Meta{Procedure: procedure, RequestID: id},
				}, stream, h, s.opts.interceptors...)
			if err != nil {
				writeFn(toErrorFrame(id, err, s.opts.errorMapper))
				return
			}
			writeFn(outboundFrame{Type: frameTypeClose, ID: id})
		},
	}
}

// HandleClientStream registers a client-streaming RPC handler for the given procedure.
func HandleClientStream[Req, Res any](
	s *Server,
	procedure string,
	h func(context.Context, procframe.ClientStream[Req]) (*procframe.Response[Res], error),
) {
	spec := procframe.CallSpec{
		Procedure: procedure,
		Transport: procframe.TransportWS,
		Shape:     procframe.CallShapeClientStream,
	}
	s.handlers[procedure] = &handlerEntry{
		shape: procframe.CallShapeClientStream,
		run: func(ctx context.Context, id string, recvCh <-chan json.RawMessage, writeFn func(outboundFrame)) {
			stream := &wsClientStream[Req]{
				getCtx: func() context.Context { return ctx },
				recvCh: recvCh,
			}
			resp, err := procframe.InvokeClientStream(
				ctx, spec, stream, h, s.opts.interceptors...,
			)
			if err != nil {
				writeFn(toErrorFrame(id, err, s.opts.errorMapper))
				return
			}
			writeResponseAndClose(id, resp, writeFn, s.opts.errorMapper)
		},
	}
}

// HandleBidi registers a bidirectional streaming RPC handler for the given procedure.
func HandleBidi[Req, Res any](
	s *Server,
	procedure string,
	h func(context.Context, procframe.BidiStream[Req, Res]) error,
) {
	s.handlers[procedure] = &handlerEntry{
		shape: procframe.CallShapeBidi,
		run: func(ctx context.Context, id string, recvCh <-chan json.RawMessage, writeFn func(outboundFrame)) {
			stream := &wsBidiStream[Req, Res]{
				getCtx:  func() context.Context { return ctx },
				recvCh:  recvCh,
				id:      id,
				writeFn: writeFn,
			}
			err := procframe.InvokeBidi(
				ctx,
				procframe.CallSpec{
					Procedure: procedure,
					Transport: procframe.TransportWS,
					Shape:     procframe.CallShapeBidi,
				},
				stream,
				h,
				s.opts.interceptors...,
			)
			if err != nil {
				writeFn(toErrorFrame(id, err, s.opts.errorMapper))
				return
			}
			writeFn(outboundFrame{Type: frameTypeClose, ID: id})
		},
	}
}

// recvExactlyOne reads one message from recvCh.
// Returns the payload and true, or nil and false if channel closed before a message.
func recvExactlyOne(recvCh <-chan json.RawMessage) (json.RawMessage, bool) {
	payload, ok := <-recvCh
	return payload, ok
}

// drainAndVerifyEmpty reads one more value from recvCh to check that
// the channel is closed (EOF). Returns true if EOF, false if extra
// messages were found (drains all remaining).
func drainAndVerifyEmpty(recvCh <-chan json.RawMessage) bool {
	extra, hasExtra := <-recvCh
	if !hasExtra {
		return true
	}
	_ = extra
	//nolint:revive // drain remaining messages to avoid goroutine leak
	for range recvCh {
	}
	return false
}

// writeResponseAndClose marshals a response, writes a message frame and
// a close frame. Errors are converted to error frames.
func writeResponseAndClose[Res any](
	id string,
	resp *procframe.Response[Res],
	writeFn func(outboundFrame),
	mapper procframe.ErrorMapper,
) {
	if resp == nil || resp.Msg == nil {
		writeFn(toErrorFrame(
			id,
			procframe.NewError(procframe.CodeInternal, "handler returned nil response"),
			mapper,
		))
		return
	}
	data, err := marshalProto(resp.Msg)
	if err != nil {
		writeFn(toErrorFrame(
			id,
			procframe.NewError(procframe.CodeInternal, err.Error()),
			mapper,
		))
		return
	}
	writeFn(outboundFrame{Type: frameTypeMessage, ID: id, Payload: data})
	writeFn(outboundFrame{Type: frameTypeClose, ID: id})
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
	var mu sync.Mutex
	sessions := make(map[string]*sessionInfo)
	var handleWG sync.WaitGroup

	s.readLoop(ctx, conn, sem, sessions, &mu, writeFn, &handleWG)

	cancel() // Cancel handler contexts on disconnect.
	closeAllSessions(sessions, &mu)
	handleWG.Wait()
	close(writeCh)
	writeWG.Wait()
}

// readLoop reads frames from the WS connection and dispatches them.
func (s *Server) readLoop(
	ctx context.Context,
	conn *websocket.Conn,
	sem chan struct{},
	sessions map[string]*sessionInfo,
	mu *sync.Mutex,
	writeFn func(outboundFrame),
	handleWG *sync.WaitGroup,
) {
	for {
		_, data, rErr := conn.Read(ctx)
		if rErr != nil {
			return
		}
		var frame inboundFrame
		if uErr := json.Unmarshal(data, &frame); uErr != nil {
			continue
		}
		switch frame.Type {
		case frameTypeOpen:
			s.handleOpen(ctx, frame, sem, sessions, mu, writeFn, handleWG)
		case frameTypeMessage:
			s.handleMessage(frame, sessions, mu, writeFn)
		case frameTypeClose:
			s.handleClose(frame, sessions, mu)
		case frameTypeCancel:
			s.handleCancel(frame, sessions, mu)
		}
	}
}

// closeAllSessions closes all remaining session recvCh channels and
// cancels contexts to unblock handlers waiting on Receive.
func closeAllSessions(sessions map[string]*sessionInfo, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()
	for _, sess := range sessions {
		sess.cancel()
		safeCloseRecvCh(sess.recvCh)
	}
}

// safeCloseRecvCh closes the channel, recovering from panic if already closed.
func safeCloseRecvCh(ch chan json.RawMessage) {
	defer func() { recover() }() //nolint:errcheck // intentional panic recovery for double-close
	close(ch)
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

// handleOpen processes an "open" frame: validates the procedure/shape,
// checks the semaphore, creates a session, and starts the handler goroutine.
func (s *Server) handleOpen(
	ctx context.Context,
	frame inboundFrame,
	sem chan struct{},
	sessions map[string]*sessionInfo,
	mu *sync.Mutex,
	writeFn func(outboundFrame),
	handleWG *sync.WaitGroup,
) {
	entry, ok := s.handlers[frame.Procedure]
	if !ok {
		writeFn(outboundFrame{
			Type: frameTypeError,
			ID:   frame.ID,
			Error: &errorDetail{
				Code:    string(procframe.CodeNotFound),
				Message: "unknown procedure: " + frame.Procedure,
			},
		})
		return
	}

	if procframe.CallShape(frame.Shape) != entry.shape {
		writeFn(outboundFrame{
			Type: frameTypeError,
			ID:   frame.ID,
			Error: &errorDetail{
				Code:    string(procframe.CodeInvalidArgument),
				Message: fmt.Sprintf("shape mismatch: want %s, got %s", entry.shape, frame.Shape),
			},
		})
		return
	}

	select {
	case sem <- struct{}{}:
	default:
		writeFn(outboundFrame{
			Type: frameTypeError,
			ID:   frame.ID,
			Error: &errorDetail{
				Code:      string(procframe.CodeUnavailable),
				Message:   "max inflight exceeded",
				Retryable: true,
			},
		})
		return
	}

	if err := s.startSession(ctx, frame, sessions, mu, sem, writeFn, handleWG, entry); err != nil {
		writeFn(toErrorFrame(frame.ID, err, s.opts.errorMapper))
	}
}

// startSession creates a session, inserts it into the map, and starts the handler goroutine.
func (s *Server) startSession(
	ctx context.Context,
	frame inboundFrame,
	sessions map[string]*sessionInfo,
	mu *sync.Mutex,
	sem chan struct{},
	writeFn func(outboundFrame),
	handleWG *sync.WaitGroup,
	entry *handlerEntry,
) error {
	sessCtx, sessCancel := context.WithCancel(ctx)
	recvCh := make(chan json.RawMessage, recvChBufSize)
	sess := &sessionInfo{
		cancel: sessCancel,
		recvCh: recvCh,
	}

	mu.Lock()
	if _, dup := sessions[frame.ID]; dup {
		mu.Unlock()
		<-sem
		sessCancel()
		return procframe.NewError(procframe.CodeAlreadyExists, "duplicate session id: "+frame.ID)
	}
	sessions[frame.ID] = sess
	mu.Unlock()

	handleWG.Go(func() {
		defer func() {
			mu.Lock()
			delete(sessions, frame.ID)
			mu.Unlock()
			<-sem
			sessCancel()
		}()
		entry.run(sessCtx, frame.ID, recvCh, writeFn)
	})
	return nil
}

// handleMessage routes a "message" frame's payload to the session's recvCh.
func (s *Server) handleMessage(
	frame inboundFrame,
	sessions map[string]*sessionInfo,
	mu *sync.Mutex,
	writeFn func(outboundFrame),
) {
	mu.Lock()
	sess, ok := sessions[frame.ID]
	if ok && sess.closed {
		mu.Unlock()
		return // Session already closed; ignore late message.
	}
	mu.Unlock()
	if !ok {
		writeFn(outboundFrame{
			Type: frameTypeError,
			ID:   frame.ID,
			Error: &errorDetail{
				Code:    string(procframe.CodeNotFound),
				Message: "session not found: " + frame.ID,
			},
		})
		return
	}
	select {
	case sess.recvCh <- frame.Payload:
	default:
		// Channel full; best-effort drop.
	}
}

// handleClose processes a "close" frame by closing the session's recvCh
// to signal EOF to the handler. The session remains in the map (with the
// closed flag set) so that handleCancel can still reach it.
func (s *Server) handleClose(
	frame inboundFrame,
	sessions map[string]*sessionInfo,
	mu *sync.Mutex,
) {
	mu.Lock()
	sess, ok := sessions[frame.ID]
	if ok {
		sess.closed = true
	}
	mu.Unlock()
	if !ok {
		return
	}
	safeCloseRecvCh(sess.recvCh)
}

// handleCancel processes a "cancel" frame by cancelling the session context.
func (s *Server) handleCancel(
	frame inboundFrame,
	sessions map[string]*sessionInfo,
	mu *sync.Mutex,
) {
	mu.Lock()
	sess, ok := sessions[frame.ID]
	mu.Unlock()
	if !ok {
		return
	}
	sess.cancel()
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
