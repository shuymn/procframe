// Package connect provides a Connect protocol transport for procframe services.
package connect

import (
	"context"
	"errors"
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
				Meta: procframe.Meta{Procedure: procedure},
			}
			pResp, err := procframe.InvokeUnary(
				ctx,
				procframe.CallSpec{
					Procedure:  procedure,
					Transport:  procframe.TransportConnect,
					StreamType: procframe.StreamTypeUnary,
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
				Meta: procframe.Meta{Procedure: procedure},
			}
			adapter := &serverStream[Res]{getCtx: func() context.Context { return ctx }, stream: stream}
			if err := procframe.InvokeServerStream(
				ctx,
				procframe.CallSpec{
					Procedure:  procedure,
					Transport:  procframe.TransportConnect,
					StreamType: procframe.StreamTypeServerStream,
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

// serverStream adapts a [connectrpc.ServerStream] to [procframe.ServerStream].
type serverStream[Res any] struct {
	getCtx func() context.Context
	stream *connectrpc.ServerStream[Res]
}

func (s *serverStream[Res]) Context() context.Context {
	return s.getCtx()
}

func (s *serverStream[Res]) Send(resp *procframe.Response[Res]) error {
	return s.stream.Send(resp.Msg)
}
