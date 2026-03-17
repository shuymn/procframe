package procframe

import (
	"context"
	"io"
)

// Transport identifies the boundary executing a handler call.
type Transport string

const (
	TransportCLI     Transport = "cli"
	TransportConnect Transport = "connect"
	TransportWS      Transport = "ws"
)

// CallShape identifies the RPC shape for a handler call.
type CallShape string

const (
	CallShapeUnary        CallShape = "unary"
	CallShapeClientStream CallShape = "client_stream"
	CallShapeServerStream CallShape = "server_stream"
	CallShapeBidi         CallShape = "bidi"
)

// CallSpec describes a transport call seen by middleware.
type CallSpec struct {
	Procedure string
	Transport Transport
	Shape     CallShape
}

// AnyRequest is the middleware-facing request view.
type AnyRequest interface {
	Any() any
	Meta() Meta
	Spec() CallSpec
}

// AnyResponse is the middleware-facing response view.
type AnyResponse interface {
	Any() any
	Meta() Meta
}

// Conn is the generic connection surface seen by middleware.
// It provides shape-independent Receive/Send operations.
// Middleware composes behavior by wrapping a Conn and delegating
// Receive/Send to the inner Conn (the conn-decorator pattern).
type Conn interface {
	Context() context.Context
	Spec() CallSpec
	Receive() (AnyRequest, error)
	Send(AnyResponse) error
}

// HandlerFunc is the generic handler invocation shape used by middleware.
type HandlerFunc func(context.Context, Conn) error

// Interceptor composes cross-cutting behavior around handler execution.
type Interceptor interface {
	Wrap(HandlerFunc) HandlerFunc
}

// InterceptorFunc is a function that implements [Interceptor].
type InterceptorFunc func(HandlerFunc) HandlerFunc

func (f InterceptorFunc) Wrap(next HandlerFunc) HandlerFunc {
	if f == nil {
		return next
	}
	return f(next)
}

// NewAnyResponse constructs a middleware-visible response from an arbitrary message.
func NewAnyResponse(msg any) AnyResponse {
	return &responseValue{msg: msg}
}

// NewAnyResponseWithMeta constructs a middleware-visible response with explicit metadata.
func NewAnyResponseWithMeta(msg any, meta Meta) AnyResponse {
	return &responseValue{msg: msg, meta: meta}
}

// InvokeUnary executes a typed unary handler through the interceptor chain.
func InvokeUnary[Req, Res any](
	ctx context.Context,
	spec CallSpec,
	req *Request[Req],
	handler func(context.Context, *Request[Req]) (*Response[Res], error),
	interceptors ...Interceptor,
) (*Response[Res], error) {
	if !hasActiveInterceptors(interceptors) {
		return handler(ctx, req)
	}

	conn := &unaryConn[Req, Res]{
		getCtx: func() context.Context { return ctx },
		spec:   spec,
		req:    req,
	}

	inner := HandlerFunc(func(ctx context.Context, c Conn) error {
		anyReq, err := c.Receive()
		if err != nil {
			return err
		}
		typedReq, err := castRequest[Req](anyReq)
		if err != nil {
			return err
		}
		resp, err := handler(ctx, typedReq)
		if err != nil {
			return err
		}
		if resp == nil {
			return nil
		}
		return c.Send(&responseView[Res]{resp: resp})
	})

	if err := chainInterceptors(inner, interceptors)(ctx, conn); err != nil {
		return nil, err
	}
	return conn.result, nil
}

// InvokeClientStream executes a typed client-stream handler through the interceptor chain.
func InvokeClientStream[Req, Res any](
	ctx context.Context,
	spec CallSpec,
	stream ClientStream[Req],
	handler func(context.Context, ClientStream[Req]) (*Response[Res], error),
	interceptors ...Interceptor,
) (*Response[Res], error) {
	conn := &clientStreamConn[Req, Res]{
		spec:   spec,
		stream: stream,
	}

	inner := HandlerFunc(func(ctx context.Context, c Conn) error {
		typedStream := &connClientStream[Req]{conn: c}
		resp, err := handler(ctx, typedStream)
		if err != nil {
			return err
		}
		if resp == nil {
			return nil
		}
		return c.Send(&responseView[Res]{resp: resp})
	})

	if err := chainInterceptors(inner, interceptors)(ctx, conn); err != nil {
		return nil, err
	}
	return conn.result, nil
}

// InvokeServerStream executes a typed server-stream handler through the interceptor chain.
func InvokeServerStream[Req, Res any](
	ctx context.Context,
	spec CallSpec,
	req *Request[Req],
	stream ServerStream[Res],
	handler func(context.Context, *Request[Req], ServerStream[Res]) error,
	interceptors ...Interceptor,
) error {
	conn := &serverStreamConn[Req, Res]{
		spec:   spec,
		req:    req,
		stream: stream,
	}

	inner := HandlerFunc(func(ctx context.Context, c Conn) error {
		anyReq, err := c.Receive()
		if err != nil {
			return err
		}
		typedReq, err := castRequest[Req](anyReq)
		if err != nil {
			return err
		}
		typedStream := &connServerStream[Res]{conn: c}
		return handler(ctx, typedReq, typedStream)
	})

	return chainInterceptors(inner, interceptors)(ctx, conn)
}

// InvokeBidi executes a typed bidi-stream handler through the interceptor chain.
func InvokeBidi[Req, Res any](
	ctx context.Context,
	spec CallSpec,
	stream BidiStream[Req, Res],
	handler func(context.Context, BidiStream[Req, Res]) error,
	interceptors ...Interceptor,
) error {
	conn := &bidiConn[Req, Res]{
		spec:   spec,
		stream: stream,
	}

	inner := HandlerFunc(func(ctx context.Context, c Conn) error {
		typedStream := &connBidiStream[Req, Res]{conn: c}
		return handler(ctx, typedStream)
	})

	return chainInterceptors(inner, interceptors)(ctx, conn)
}

func chainInterceptors(inner HandlerFunc, interceptors []Interceptor) HandlerFunc {
	next := inner
	for i := len(interceptors) - 1; i >= 0; i-- {
		if interceptors[i] == nil {
			continue
		}
		next = interceptors[i].Wrap(next)
	}
	return next
}

func hasActiveInterceptors(interceptors []Interceptor) bool {
	for _, interceptor := range interceptors {
		if interceptor != nil {
			return true
		}
	}
	return false
}

// --- request / response views ---

type requestView[T any] struct {
	spec CallSpec
	req  *Request[T]
}

func (r *requestView[T]) Any() any {
	if r == nil || r.req == nil {
		return nil
	}
	return r.req.Msg
}

func (r *requestView[T]) Meta() Meta {
	if r == nil || r.req == nil {
		return Meta{}
	}
	return r.req.Meta
}

func (r *requestView[T]) Spec() CallSpec {
	if r == nil {
		return CallSpec{}
	}
	return r.spec
}

type responseView[T any] struct {
	resp *Response[T]
}

func (r *responseView[T]) Any() any {
	if r == nil || r.resp == nil {
		return nil
	}
	return r.resp.Msg
}

func (r *responseView[T]) Meta() Meta {
	if r == nil || r.resp == nil {
		return Meta{}
	}
	return r.resp.Meta
}

type responseValue struct {
	msg  any
	meta Meta
}

func (r *responseValue) Any() any {
	if r == nil {
		return nil
	}
	return r.msg
}

func (r *responseValue) Meta() Meta {
	if r == nil {
		return Meta{}
	}
	return r.meta
}

// --- Conn implementations ---

// unaryConn bridges a unary typed handler to the generic Conn surface.
type unaryConn[Req, Res any] struct {
	getCtx   func() context.Context
	spec     CallSpec
	req      *Request[Req]
	received bool
	result   *Response[Res]
}

func (c *unaryConn[Req, Res]) Context() context.Context { return c.getCtx() }
func (c *unaryConn[Req, Res]) Spec() CallSpec           { return c.spec }

func (c *unaryConn[Req, Res]) Receive() (AnyRequest, error) {
	if c.received {
		return nil, io.EOF
	}
	c.received = true
	return &requestView[Req]{spec: c.spec, req: c.req}, nil
}

func (c *unaryConn[Req, Res]) Send(resp AnyResponse) error {
	if resp == nil {
		return nil
	}
	typedResp, err := castResponse[Res](resp)
	if err != nil {
		return err
	}
	c.result = typedResp
	return nil
}

// clientStreamConn bridges a client-stream typed handler to the generic Conn surface.
type clientStreamConn[Req, Res any] struct {
	spec   CallSpec
	stream ClientStream[Req]
	result *Response[Res]
}

func (c *clientStreamConn[Req, Res]) Context() context.Context { return c.stream.Context() }
func (c *clientStreamConn[Req, Res]) Spec() CallSpec           { return c.spec }

func (c *clientStreamConn[Req, Res]) Receive() (AnyRequest, error) {
	req, err := c.stream.Receive()
	if err != nil {
		return nil, err
	}
	return &requestView[Req]{spec: c.spec, req: req}, nil
}

func (c *clientStreamConn[Req, Res]) Send(resp AnyResponse) error {
	if resp == nil {
		return nil
	}
	typedResp, err := castResponse[Res](resp)
	if err != nil {
		return err
	}
	c.result = typedResp
	return nil
}

// serverStreamConn bridges a server-stream typed handler to the generic Conn surface.
type serverStreamConn[Req, Res any] struct {
	spec     CallSpec
	req      *Request[Req]
	received bool
	stream   ServerStream[Res]
}

func (c *serverStreamConn[Req, Res]) Context() context.Context { return c.stream.Context() }
func (c *serverStreamConn[Req, Res]) Spec() CallSpec           { return c.spec }

func (c *serverStreamConn[Req, Res]) Receive() (AnyRequest, error) {
	if c.received {
		return nil, io.EOF
	}
	c.received = true
	return &requestView[Req]{spec: c.spec, req: c.req}, nil
}

func (c *serverStreamConn[Req, Res]) Send(resp AnyResponse) error {
	typedResp, err := castResponse[Res](resp)
	if err != nil {
		return err
	}
	if typedResp == nil || typedResp.Msg == nil {
		return NewError(CodeInternal, "handler sent nil response")
	}
	return c.stream.Send(typedResp)
}

// bidiConn bridges a bidi-stream typed handler to the generic Conn surface.
type bidiConn[Req, Res any] struct {
	spec   CallSpec
	stream BidiStream[Req, Res]
}

func (c *bidiConn[Req, Res]) Context() context.Context { return c.stream.Context() }
func (c *bidiConn[Req, Res]) Spec() CallSpec           { return c.spec }

func (c *bidiConn[Req, Res]) Receive() (AnyRequest, error) {
	req, err := c.stream.Receive()
	if err != nil {
		return nil, err
	}
	return &requestView[Req]{spec: c.spec, req: req}, nil
}

func (c *bidiConn[Req, Res]) Send(resp AnyResponse) error {
	typedResp, err := castResponse[Res](resp)
	if err != nil {
		return err
	}
	if typedResp == nil || typedResp.Msg == nil {
		return NewError(CodeInternal, "handler sent nil response")
	}
	return c.stream.Send(typedResp)
}

// --- typed stream adapters (Conn → typed handler interface) ---

// connClientStream adapts a Conn's Receive to a typed ClientStream.
type connClientStream[Req any] struct {
	conn Conn
}

func (s *connClientStream[Req]) Context() context.Context { return s.conn.Context() }

func (s *connClientStream[Req]) Receive() (*Request[Req], error) {
	anyReq, err := s.conn.Receive()
	if err != nil {
		return nil, err
	}
	return castRequest[Req](anyReq)
}

// connServerStream adapts a Conn's Send to a typed ServerStream.
type connServerStream[Res any] struct {
	conn Conn
}

func (s *connServerStream[Res]) Context() context.Context { return s.conn.Context() }

func (s *connServerStream[Res]) Send(resp *Response[Res]) error {
	if resp == nil {
		return NewError(CodeInternal, "handler sent nil response")
	}
	return s.conn.Send(&responseView[Res]{resp: resp})
}

// connBidiStream adapts a Conn's Receive/Send to a typed BidiStream.
type connBidiStream[Req, Res any] struct {
	conn Conn
}

func (s *connBidiStream[Req, Res]) Context() context.Context { return s.conn.Context() }

func (s *connBidiStream[Req, Res]) Receive() (*Request[Req], error) {
	anyReq, err := s.conn.Receive()
	if err != nil {
		return nil, err
	}
	return castRequest[Req](anyReq)
}

func (s *connBidiStream[Req, Res]) Send(resp *Response[Res]) error {
	if resp == nil {
		return NewError(CodeInternal, "handler sent nil response")
	}
	return s.conn.Send(&responseView[Res]{resp: resp})
}

// --- cast helpers ---

func castRequest[T any](req AnyRequest) (*Request[T], error) {
	if req == nil {
		return nil, NewError(CodeInternal, "interceptor passed nil request")
	}
	if typed, ok := req.(*requestView[T]); ok {
		return typed.req, nil
	}
	msg, ok := req.Any().(*T)
	if !ok {
		return nil, Errorf(CodeInternal, "interceptor passed unexpected request type %T", req.Any())
	}
	return &Request[T]{
		Msg:  msg,
		Meta: req.Meta(),
	}, nil
}

func castResponse[T any](resp AnyResponse) (*Response[T], error) {
	if resp == nil {
		return nil, nil
	}
	if typed, ok := resp.(*responseView[T]); ok {
		return typed.resp, nil
	}
	msg, ok := resp.Any().(*T)
	if !ok {
		return nil, Errorf(CodeInternal, "interceptor returned unexpected response type %T", resp.Any())
	}
	return &Response[T]{
		Msg:  msg,
		Meta: resp.Meta(),
	}, nil
}
