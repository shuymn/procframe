package procframe

import "context"

// Transport identifies the boundary executing a handler call.
type Transport string

const (
	TransportCLI     Transport = "cli"
	TransportConnect Transport = "connect"
	TransportWS      Transport = "ws"
)

// StreamType identifies the RPC shape for a handler call.
type StreamType string

const (
	StreamTypeUnary        StreamType = "unary"
	StreamTypeServerStream StreamType = "server_stream"
)

// CallSpec describes a transport call seen by interceptors.
type CallSpec struct {
	Procedure  string
	Transport  Transport
	StreamType StreamType
}

// AnyRequest is the interceptor-facing request view.
type AnyRequest interface {
	Any() any
	Meta() *Meta
	Spec() CallSpec
}

// AnyResponse is the interceptor-facing response view.
type AnyResponse interface {
	Any() any
	Meta() *Meta
}

// AnyServerStream is the interceptor-facing stream view.
type AnyServerStream interface {
	Context() context.Context
	Spec() CallSpec
	Send(AnyResponse) error
}

// UnaryFunc is the common unary invocation shape used by interceptors.
type UnaryFunc func(context.Context, AnyRequest) (AnyResponse, error)

// ServerStreamFunc is the common server-stream invocation shape used by interceptors.
type ServerStreamFunc func(context.Context, AnyRequest, AnyServerStream) error

// StreamSendFunc wraps an individual server-stream send.
type StreamSendFunc func(AnyResponse) error

// Interceptor composes cross-cutting behavior around handler execution.
type Interceptor interface {
	WrapUnary(UnaryFunc) UnaryFunc
	WrapServerStream(ServerStreamFunc) ServerStreamFunc
	WrapStreamSend(StreamSendFunc) StreamSendFunc
}

// UnaryInterceptorFunc wraps unary calls and leaves streaming unchanged.
type UnaryInterceptorFunc func(UnaryFunc) UnaryFunc

func (f UnaryInterceptorFunc) WrapUnary(next UnaryFunc) UnaryFunc {
	if f == nil {
		return next
	}
	return f(next)
}

func (UnaryInterceptorFunc) WrapServerStream(next ServerStreamFunc) ServerStreamFunc { return next }

func (UnaryInterceptorFunc) WrapStreamSend(next StreamSendFunc) StreamSendFunc { return next }

// ServerStreamInterceptorFunc wraps server-stream calls and leaves other paths unchanged.
type ServerStreamInterceptorFunc func(ServerStreamFunc) ServerStreamFunc

func (ServerStreamInterceptorFunc) WrapUnary(next UnaryFunc) UnaryFunc { return next }

func (f ServerStreamInterceptorFunc) WrapServerStream(next ServerStreamFunc) ServerStreamFunc {
	if f == nil {
		return next
	}
	return f(next)
}

func (ServerStreamInterceptorFunc) WrapStreamSend(next StreamSendFunc) StreamSendFunc { return next }

// StreamSendInterceptorFunc wraps individual server-stream sends.
type StreamSendInterceptorFunc func(StreamSendFunc) StreamSendFunc

func (StreamSendInterceptorFunc) WrapUnary(next UnaryFunc) UnaryFunc { return next }

func (StreamSendInterceptorFunc) WrapServerStream(next ServerStreamFunc) ServerStreamFunc {
	return next
}

func (f StreamSendInterceptorFunc) WrapStreamSend(next StreamSendFunc) StreamSendFunc {
	if f == nil {
		return next
	}
	return f(next)
}

// NewAnyResponse constructs an interceptor-visible response from an arbitrary message.
func NewAnyResponse(msg any) AnyResponse {
	return &responseValue{msg: msg}
}

// NewAnyResponseWithMeta constructs an interceptor-visible response with explicit metadata.
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
	next := func(ctx context.Context, req AnyRequest) (AnyResponse, error) {
		typedReq, err := castRequest[Req](req)
		if err != nil {
			return nil, err
		}
		resp, err := handler(ctx, typedReq)
		if resp == nil || err != nil {
			return nilResponse[Res](resp), err
		}
		return &responseView[Res]{resp: resp}, nil
	}

	for i := len(interceptors) - 1; i >= 0; i-- {
		if interceptors[i] == nil {
			continue
		}
		next = interceptors[i].WrapUnary(next)
	}

	resp, err := next(ctx, &requestView[Req]{spec: spec, req: req})
	if err != nil {
		return nil, err
	}
	return castResponse[Res](resp)
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
	send := func(resp AnyResponse) error {
		typedResp, err := castResponse[Res](resp)
		if err != nil {
			return err
		}
		if typedResp == nil || typedResp.Msg == nil {
			return NewError(CodeInternal, "handler sent nil response")
		}
		return stream.Send(typedResp)
	}
	for i := len(interceptors) - 1; i >= 0; i-- {
		if interceptors[i] == nil {
			continue
		}
		send = interceptors[i].WrapStreamSend(send)
	}

	next := func(ctx context.Context, req AnyRequest, stream AnyServerStream) error {
		typedReq, err := castRequest[Req](req)
		if err != nil {
			return err
		}
		return handler(ctx, typedReq, &typedStream[Res]{stream: stream})
	}
	for i := len(interceptors) - 1; i >= 0; i-- {
		if interceptors[i] == nil {
			continue
		}
		next = interceptors[i].WrapServerStream(next)
	}

	return next(ctx, &requestView[Req]{spec: spec, req: req}, &anyStream{
		getCtx: stream.Context,
		spec:   spec,
		send:   send,
	})
}

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

func (r *requestView[T]) Meta() *Meta {
	if r == nil || r.req == nil {
		return nil
	}
	return &r.req.Meta
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

func (r *responseView[T]) Meta() *Meta {
	if r == nil || r.resp == nil {
		return nil
	}
	return &r.resp.Meta
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

func (r *responseValue) Meta() *Meta {
	if r == nil {
		return nil
	}
	return &r.meta
}

type anyStream struct {
	getCtx func() context.Context
	spec   CallSpec
	send   StreamSendFunc
}

func (s *anyStream) Context() context.Context {
	if s == nil {
		return nil
	}
	return s.getCtx()
}

func (s *anyStream) Spec() CallSpec {
	if s == nil {
		return CallSpec{}
	}
	return s.spec
}

func (s *anyStream) Send(resp AnyResponse) error {
	if s == nil {
		return NewError(CodeInternal, "stream is nil")
	}
	return s.send(resp)
}

type typedStream[T any] struct {
	stream AnyServerStream
}

func (s *typedStream[T]) Context() context.Context {
	return s.stream.Context()
}

func (s *typedStream[T]) Send(resp *Response[T]) error {
	return s.stream.Send(nilResponse[T](resp))
}

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
	meta := Meta{}
	if req.Meta() != nil {
		meta = *req.Meta()
	}
	return &Request[T]{
		Msg:  msg,
		Meta: meta,
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
	meta := Meta{}
	if resp.Meta() != nil {
		meta = *resp.Meta()
	}
	return &Response[T]{
		Msg:  msg,
		Meta: meta,
	}, nil
}

func nilResponse[T any](resp *Response[T]) AnyResponse {
	if resp == nil {
		return nil
	}
	return &responseView[T]{resp: resp}
}
