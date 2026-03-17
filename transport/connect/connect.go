// Package connect provides a Connect protocol transport for procframe services.
package connect

import (
	"context"
	"errors"
	"io"
	"net/http"

	connectrpc "connectrpc.com/connect"

	"github.com/shuymn/procframe"
)

// Option configures Connect transport behavior.
type Option func(*options)

type options struct {
	errorMapper  procframe.ErrorMapper
	interceptors []procframe.Interceptor
}

// WithErrorMapper sets the boundary mapper used to classify errors
// that are not already wrapped as [procframe.StatusError].
func WithErrorMapper(mapper procframe.ErrorMapper) Option {
	return func(o *options) { o.errorMapper = mapper }
}

// WithInterceptors sets the interceptor chain applied to handler execution.
func WithInterceptors(interceptors ...procframe.Interceptor) Option {
	return func(o *options) {
		o.interceptors = append([]procframe.Interceptor(nil), interceptors...)
	}
}

func buildOptions(opts []Option) *options {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// NewUnaryHandler wraps a procframe unary handler function as a Connect
// HTTP handler. It returns the procedure path and the handler, suitable
// for registering on an [http.ServeMux].
func NewUnaryHandler[Req, Res any](
	procedure string,
	handler func(context.Context, *procframe.Request[Req]) (*procframe.Response[Res], error),
	opts ...Option,
) (string, http.Handler) {
	o := buildOptions(opts)
	return procedure, connectrpc.NewUnaryHandler(
		procedure,
		func(ctx context.Context, req *connectrpc.Request[Req]) (*connectrpc.Response[Res], error) {
			pReq := &procframe.Request[Req]{
				Msg:  req.Msg,
				Meta: &procframe.Meta{Procedure: procedure},
			}
			pResp, err := procframe.InvokeUnary(
				ctx,
				&procframe.CallSpec{
					Procedure: procedure,
					Transport: procframe.TransportConnect,
					Shape:     procframe.CallShapeUnary,
				},
				pReq,
				handler,
				o.interceptors...,
			)
			if err != nil {
				return nil, toConnectError(err, o.errorMapper)
			}
			if pResp == nil || pResp.Msg == nil {
				return nil, connectrpc.NewError(connectrpc.CodeInternal, errors.New("handler returned nil response"))
			}
			return connectrpc.NewResponse(pResp.Msg), nil
		},
	)
}

// NewClientStreamHandler wraps a procframe client-streaming handler function
// as a Connect HTTP handler. It returns the procedure path and the handler.
func NewClientStreamHandler[Req, Res any](
	procedure string,
	handler func(context.Context, procframe.ClientStream[Req]) (*procframe.Response[Res], error),
	opts ...Option,
) (string, http.Handler) {
	o := buildOptions(opts)
	return procedure, connectrpc.NewClientStreamHandler(
		procedure,
		func(ctx context.Context, stream *connectrpc.ClientStream[Req]) (*connectrpc.Response[Res], error) {
			adapter := &clientStream[Req]{getCtx: func() context.Context { return ctx }, stream: stream}
			pResp, err := procframe.InvokeClientStream(
				ctx,
				&procframe.CallSpec{
					Procedure: procedure,
					Transport: procframe.TransportConnect,
					Shape:     procframe.CallShapeClientStream,
				},
				adapter,
				handler,
				o.interceptors...,
			)
			if err != nil {
				return nil, toConnectError(err, o.errorMapper)
			}
			if pResp == nil || pResp.Msg == nil {
				return nil, connectrpc.NewError(connectrpc.CodeInternal, errors.New("handler returned nil response"))
			}
			return connectrpc.NewResponse(pResp.Msg), nil
		},
	)
}

// NewServerStreamHandler wraps a procframe server-streaming handler function
// as a Connect HTTP handler. It returns the procedure path and the handler.
func NewServerStreamHandler[Req, Res any](
	procedure string,
	handler func(context.Context, *procframe.Request[Req], procframe.ServerStream[Res]) error,
	opts ...Option,
) (string, http.Handler) {
	o := buildOptions(opts)
	return procedure, connectrpc.NewServerStreamHandler(
		procedure,
		func(ctx context.Context, req *connectrpc.Request[Req], stream *connectrpc.ServerStream[Res]) error {
			pReq := &procframe.Request[Req]{
				Msg:  req.Msg,
				Meta: &procframe.Meta{Procedure: procedure},
			}
			adapter := &serverStream[Res]{getCtx: func() context.Context { return ctx }, stream: stream}
			if err := procframe.InvokeServerStream(
				ctx,
				&procframe.CallSpec{
					Procedure: procedure,
					Transport: procframe.TransportConnect,
					Shape:     procframe.CallShapeServerStream,
				},
				pReq,
				adapter,
				handler,
				o.interceptors...,
			); err != nil {
				return toConnectError(err, o.errorMapper)
			}
			return nil
		},
	)
}

// NewBidiStreamHandler wraps a procframe bidirectional-streaming handler function
// as a Connect HTTP handler. It returns the procedure path and the handler.
func NewBidiStreamHandler[Req, Res any](
	procedure string,
	handler func(context.Context, procframe.BidiStream[Req, Res]) error,
	opts ...Option,
) (string, http.Handler) {
	o := buildOptions(opts)
	return procedure, connectrpc.NewBidiStreamHandler(
		procedure,
		func(ctx context.Context, stream *connectrpc.BidiStream[Req, Res]) error {
			adapter := &bidiStream[Req, Res]{getCtx: func() context.Context { return ctx }, stream: stream}
			if err := procframe.InvokeBidi(
				ctx,
				&procframe.CallSpec{
					Procedure: procedure,
					Transport: procframe.TransportConnect,
					Shape:     procframe.CallShapeBidi,
				},
				adapter,
				handler,
				o.interceptors...,
			); err != nil {
				return toConnectError(err, o.errorMapper)
			}
			return nil
		},
	)
}

// --- stream adapters ---

// clientStream adapts a [connectrpc.ClientStream] to [procframe.ClientStream].
type clientStream[Req any] struct {
	getCtx func() context.Context
	stream *connectrpc.ClientStream[Req]
}

func (s *clientStream[Req]) Context() context.Context { return s.getCtx() }

func (s *clientStream[Req]) Receive() (*procframe.Request[Req], error) {
	if !s.stream.Receive() {
		if err := s.stream.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	return &procframe.Request[Req]{Msg: s.stream.Msg()}, nil
}

// serverStream adapts a [connectrpc.ServerStream] to [procframe.ServerStream].
type serverStream[Res any] struct {
	getCtx func() context.Context
	stream *connectrpc.ServerStream[Res]
}

func (s *serverStream[Res]) Context() context.Context { return s.getCtx() }

func (s *serverStream[Res]) Send(resp *procframe.Response[Res]) error {
	return s.stream.Send(resp.Msg)
}

// bidiStream adapts a [connectrpc.BidiStream] to [procframe.BidiStream].
type bidiStream[Req, Res any] struct {
	getCtx func() context.Context
	stream *connectrpc.BidiStream[Req, Res]
}

func (s *bidiStream[Req, Res]) Context() context.Context { return s.getCtx() }

func (s *bidiStream[Req, Res]) Receive() (*procframe.Request[Req], error) {
	msg, err := s.stream.Receive()
	if err != nil {
		return nil, err
	}
	return &procframe.Request[Req]{Msg: msg}, nil
}

func (s *bidiStream[Req, Res]) Send(resp *procframe.Response[Res]) error {
	return s.stream.Send(resp.Msg)
}
