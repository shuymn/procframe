package ws

import (
	"context"

	"github.com/shuymn/procframe"
)

// CallUnary performs a unary RPC on the multiplexed connection.
func CallUnary[Req, Res any](
	ctx context.Context,
	c *Conn,
	procedure string,
	req *Req,
) (*Res, error) {
	id, recvCh := c.newSession()
	defer c.removeSession(id)

	if err := sendRequest(c, id, procedure, string(procframe.CallShapeUnary), req); err != nil {
		return nil, err
	}

	res, err := receiveResponse[Res](ctx, c, recvCh)
	if err != nil && ctx.Err() != nil {
		//nolint:errcheck // best-effort cancel; connection may already be closed
		c.sendCancel(id)
	}
	return res, err
}

// CallServerStream opens a server-streaming RPC and returns a stream
// that yields successive responses.
func CallServerStream[Req, Res any](
	ctx context.Context,
	c *Conn,
	procedure string,
	req *Req,
) (ServerStream[Res], error) {
	id, recvCh := c.newSession()

	if err := sendRequest(c, id, procedure, string(procframe.CallShapeServerStream), req); err != nil {
		c.removeSession(id)
		return nil, err
	}

	//nolint:gosec // streamCancel is stored in the stream and called by Close
	streamCtx, streamCancel := context.WithCancel(ctx)
	return &clientServerStream[Res]{
		getCtx: func() context.Context { return streamCtx },
		cancel: streamCancel,
		conn:   c,
		sessID: id,
		recvCh: recvCh,
	}, nil
}

// CallClientStream opens a client-streaming RPC and returns a stream
// for sending messages and receiving the final response.
func CallClientStream[Req, Res any](
	ctx context.Context,
	c *Conn,
	procedure string,
) (ClientStream[Req, Res], error) {
	id, recvCh := c.newSession()

	if err := c.sendOpen(id, procedure, string(procframe.CallShapeClientStream)); err != nil {
		c.removeSession(id)
		return nil, err
	}

	return &clientClientStream[Req, Res]{
		getCtx: func() context.Context { return ctx },
		conn:   c,
		sessID: id,
		recvCh: recvCh,
	}, nil
}

// CallBidi opens a bidirectional streaming RPC and returns a stream
// for concurrent sending and receiving.
func CallBidi[Req, Res any](
	ctx context.Context,
	c *Conn,
	procedure string,
) (BidiStream[Req, Res], error) {
	id, recvCh := c.newSession()

	if err := c.sendOpen(id, procedure, string(procframe.CallShapeBidi)); err != nil {
		c.removeSession(id)
		return nil, err
	}

	//nolint:gosec // streamCancel is stored in the stream and called during cleanup
	streamCtx, streamCancel := context.WithCancel(ctx)
	return &clientBidiStream[Req, Res]{
		getCtx: func() context.Context { return streamCtx },
		cancel: streamCancel,
		conn:   c,
		sessID: id,
		recvCh: recvCh,
	}, nil
}

// sendRequest opens a session, sends the marshalled request, and closes
// the send direction. Used by unary and server-stream calls.
func sendRequest[Req any](c *Conn, id, procedure, shape string, req *Req) error {
	if err := c.sendOpen(id, procedure, shape); err != nil {
		return err
	}
	data, err := marshalProto(req)
	if err != nil {
		return err
	}
	if err := c.sendMessage(id, data); err != nil {
		return err
	}
	return c.sendClose(id)
}
