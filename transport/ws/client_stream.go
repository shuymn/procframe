package ws

import (
	"context"
	"encoding/json"
	"io"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/shuymn/procframe"
)

// ServerStream receives server-sent messages for a single RPC session.
type ServerStream[Res any] interface {
	// Receive returns the next server message, or io.EOF when the server
	// sends a close frame.
	Receive() (*Res, error)
	// Close sends a cancel frame to abort the stream.
	Close() error
}

// ClientStream sends client messages and receives a single response.
type ClientStream[Req, Res any] interface {
	// Send transmits a message to the server.
	Send(*Req) error
	// CloseAndReceive sends a close frame and waits for the server's response.
	CloseAndReceive() (*Res, error)
}

// BidiStream supports concurrent sending and receiving on a single session.
type BidiStream[Req, Res any] interface {
	// Send transmits a message to the server.
	Send(*Req) error
	// Receive returns the next server message, or io.EOF on close.
	Receive() (*Res, error)
	// CloseSend sends a close frame (send direction only). Receive may
	// still return messages until the server closes.
	CloseSend() error
}

// --- Implementations ---

type clientServerStream[Res any] struct {
	getCtx  func() context.Context
	cancel  context.CancelFunc // cancels the stream context; unblocks pending Receive
	conn    *Conn
	sessID  string
	recvCh  <-chan outboundFrame
	closed  bool
	cleanup sync.Once
	termErr error // set on first terminal error; prevents blocking on orphaned recvCh
}

func (s *clientServerStream[Res]) Receive() (*Res, error) {
	if s.termErr != nil {
		return nil, s.termErr
	}
	res, err := receiveResponse[Res](s.getCtx(), s.conn, s.recvCh)
	if err != nil {
		s.termErr = err
		s.cleanup.Do(func() { s.conn.removeSession(s.sessID) })
	}
	return res, err
}

func (s *clientServerStream[Res]) Close() error {
	s.cleanup.Do(func() { s.conn.removeSession(s.sessID) })
	s.cancel() // unblock any pending Receive; idempotent
	if s.closed {
		return nil
	}
	s.closed = true
	return s.conn.sendCancel(s.sessID)
}

type clientClientStream[Req, Res any] struct {
	getCtx  func() context.Context
	conn    *Conn
	sessID  string
	recvCh  <-chan outboundFrame
	cleanup sync.Once
}

func (s *clientClientStream[Req, Res]) Send(msg *Req) error {
	data, err := marshalProto(msg)
	if err != nil {
		return err
	}
	return s.conn.sendMessage(s.sessID, data)
}

func (s *clientClientStream[Req, Res]) CloseAndReceive() (*Res, error) {
	defer s.cleanup.Do(func() { s.conn.removeSession(s.sessID) })
	if err := s.conn.sendClose(s.sessID); err != nil {
		return nil, err
	}
	return receiveResponse[Res](s.getCtx(), s.conn, s.recvCh)
}

type clientBidiStream[Req, Res any] struct {
	getCtx  func() context.Context
	cancel  context.CancelFunc // cancels the stream context; unblocks pending Receive on teardown
	conn    *Conn
	sessID  string
	recvCh  <-chan outboundFrame
	cleanup sync.Once
	termErr error // set on first terminal error; prevents blocking on orphaned recvCh
}

func (s *clientBidiStream[Req, Res]) Send(msg *Req) error {
	data, err := marshalProto(msg)
	if err != nil {
		return err
	}
	return s.conn.sendMessage(s.sessID, data)
}

func (s *clientBidiStream[Req, Res]) Receive() (*Res, error) {
	if s.termErr != nil {
		return nil, s.termErr
	}
	res, err := receiveResponse[Res](s.getCtx(), s.conn, s.recvCh)
	if err != nil {
		s.termErr = err
		s.cleanup.Do(func() { s.conn.removeSession(s.sessID) })
	}
	return res, err
}

func (s *clientBidiStream[Req, Res]) CloseSend() error {
	return s.conn.sendClose(s.sessID)
}

// --- Helpers ---

// receiveResponse reads the next message frame from recvCh and unmarshals
// the payload. Returns io.EOF on a close frame.
func receiveResponse[Res any](
	ctx context.Context,
	conn *Conn,
	recvCh <-chan outboundFrame,
) (*Res, error) {
	for {
		select {
		case frame, ok := <-recvCh:
			if !ok {
				return nil, conn.closeErr()
			}
			if frame.Error != nil {
				return nil, toStatusError(frame.Error)
			}
			if frame.Type == frameTypeClose {
				return nil, io.EOF
			}
			if frame.Type != frameTypeMessage {
				continue
			}
			return unmarshalResponse[Res](frame.Payload)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// unmarshalResponse deserializes a JSON payload into a proto response message.
func unmarshalResponse[Res any](payload json.RawMessage) (*Res, error) {
	var res Res
	msg, ok := any(&res).(proto.Message)
	if !ok {
		return nil, procframe.Errorf(
			procframe.CodeInternal,
			"response type %T does not implement proto.Message",
			res,
		)
	}
	if err := protojson.Unmarshal(payload, msg); err != nil {
		return nil, procframe.NewError(procframe.CodeInternal, err.Error())
	}
	return &res, nil
}

// toStatusError converts an errorDetail from a server error frame to a
// *procframe.StatusError.
func toStatusError(ed *errorDetail) *procframe.StatusError {
	err := procframe.NewError(procframe.Code(ed.Code), ed.Message)
	if ed.Retryable {
		err = err.WithRetryable()
	}
	return err
}
