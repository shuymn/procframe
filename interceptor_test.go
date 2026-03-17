package procframe_test

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/shuymn/procframe"
)

type testRequest struct {
	Message string
}

type testResponse struct {
	Message string
}

// --- ServerStream helper ---

type collectingStream struct {
	getCtx func() context.Context
	sent   []*procframe.Response[testResponse]
}

func (s *collectingStream) Context() context.Context {
	return s.getCtx()
}

func (s *collectingStream) Send(resp *procframe.Response[testResponse]) error {
	s.sent = append(s.sent, resp)
	return nil
}

// --- ClientStream helper ---

type sliceClientStream struct {
	getCtx func() context.Context
	msgs   []*testRequest
	idx    int
}

func (s *sliceClientStream) Context() context.Context { return s.getCtx() }

func (s *sliceClientStream) Receive() (*procframe.Request[testRequest], error) {
	if s.idx >= len(s.msgs) {
		return nil, io.EOF
	}
	msg := s.msgs[s.idx]
	s.idx++
	return &procframe.Request[testRequest]{Msg: msg}, nil
}

// --- BidiStream helper ---

type sliceBidiStream struct {
	getCtx func() context.Context
	msgs   []*testRequest
	idx    int
	sent   []*procframe.Response[testResponse]
}

func (s *sliceBidiStream) Context() context.Context { return s.getCtx() }

func (s *sliceBidiStream) Receive() (*procframe.Request[testRequest], error) {
	if s.idx >= len(s.msgs) {
		return nil, io.EOF
	}
	msg := s.msgs[s.idx]
	s.idx++
	return &procframe.Request[testRequest]{Msg: msg}, nil
}

func (s *sliceBidiStream) Send(resp *procframe.Response[testResponse]) error {
	s.sent = append(s.sent, resp)
	return nil
}

// --- Tests ---

func TestInvokeUnary_NoInterceptor(t *testing.T) {
	t.Parallel()

	spec := procframe.CallSpec{
		Procedure: "/test.v1.EchoService/Echo",
		Transport: procframe.TransportCLI,
		Shape:     procframe.CallShapeUnary,
	}

	resp, err := procframe.InvokeUnary(
		t.Context(),
		spec,
		&procframe.Request[testRequest]{Msg: &testRequest{Message: "hello"}},
		func(_ context.Context, req *procframe.Request[testRequest]) (*procframe.Response[testResponse], error) {
			return &procframe.Response[testResponse]{
				Msg: &testResponse{Message: "echo:" + req.Msg.Message},
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Message != "echo:hello" {
		t.Fatalf("want echo:hello, got %q", resp.Msg.Message)
	}
}

func TestInvokeUnary_InterceptorOrdering(t *testing.T) {
	t.Parallel()

	spec := procframe.CallSpec{
		Procedure: "/test.v1.EchoService/Echo",
		Transport: procframe.TransportCLI,
		Shape:     procframe.CallShapeUnary,
	}
	req := &procframe.Request[testRequest]{
		Msg:  &testRequest{Message: "hello"},
		Meta: procframe.Meta{Procedure: spec.Procedure},
	}
	var events []string

	resp, err := procframe.InvokeUnary(
		t.Context(),
		spec,
		req,
		func(_ context.Context, req *procframe.Request[testRequest]) (*procframe.Response[testResponse], error) {
			events = append(events, "handler")
			if req.Msg.Message != "wrapped:hello" {
				t.Fatalf("want wrapped request, got %q", req.Msg.Message)
			}
			return &procframe.Response[testResponse]{
				Msg: &testResponse{Message: "handler"},
			}, nil
		},
		// Outer interceptor: wraps Conn to modify request on Receive
		procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
			return func(ctx context.Context, conn procframe.Conn) error {
				events = append(events, "outer before")
				if conn.Spec() != spec {
					t.Fatalf("unexpected spec: %+v", conn.Spec())
				}
				err := next(ctx, &modifyReceiveConn{
					Conn: conn,
					modify: func(req procframe.AnyRequest) {
						msg, ok := req.Any().(*testRequest)
						if !ok {
							t.Fatalf("want *testRequest, got %T", req.Any())
						}
						msg.Message = "wrapped:" + msg.Message
					},
				})
				events = append(events, "outer after")
				return err
			}
		}),
		// Inner interceptor: just observes
		procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
			return func(ctx context.Context, conn procframe.Conn) error {
				events = append(events, "inner before")
				err := next(ctx, conn)
				events = append(events, "inner after")
				return err
			}
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Message != "handler" {
		t.Fatalf("want handler response, got %q", resp.Msg.Message)
	}

	want := []string{
		"outer before",
		"inner before",
		"handler",
		"inner after",
		"outer after",
	}
	if !slices.Equal(events, want) {
		t.Fatalf("want %v, got %v", want, events)
	}
}

func TestInvokeUnary_ShortCircuit(t *testing.T) {
	t.Parallel()

	spec := procframe.CallSpec{
		Procedure: "/test.v1.EchoService/Echo",
		Transport: procframe.TransportConnect,
		Shape:     procframe.CallShapeUnary,
	}

	resp, err := procframe.InvokeUnary(
		t.Context(),
		spec,
		&procframe.Request[testRequest]{
			Msg:  &testRequest{Message: "ignored"},
			Meta: procframe.Meta{Procedure: spec.Procedure},
		},
		func(context.Context, *procframe.Request[testRequest]) (*procframe.Response[testResponse], error) {
			t.Fatal("handler must not run")
			return nil, nil
		},
		procframe.InterceptorFunc(func(_ procframe.HandlerFunc) procframe.HandlerFunc {
			return func(_ context.Context, conn procframe.Conn) error {
				return conn.Send(procframe.NewAnyResponse(&testResponse{Message: "short-circuit"}))
			}
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Message != "short-circuit" {
		t.Fatalf("want short-circuit response, got %q", resp.Msg.Message)
	}
}

func TestInvokeUnary_NilInterceptor(t *testing.T) {
	t.Parallel()

	resp, err := procframe.InvokeUnary(
		t.Context(),
		procframe.CallSpec{
			Procedure: "/test.v1.EchoService/Echo",
			Transport: procframe.TransportCLI,
			Shape:     procframe.CallShapeUnary,
		},
		&procframe.Request[testRequest]{Msg: &testRequest{Message: "hello"}},
		func(_ context.Context, _ *procframe.Request[testRequest]) (*procframe.Response[testResponse], error) {
			return &procframe.Response[testResponse]{Msg: &testResponse{Message: "ok"}}, nil
		},
		nil, // nil interceptor should be skipped
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Message != "ok" {
		t.Fatalf("want ok, got %q", resp.Msg.Message)
	}
}

func TestInvokeUnary_RejectsUnexpectedResponseType(t *testing.T) {
	t.Parallel()

	_, err := procframe.InvokeUnary(
		t.Context(),
		procframe.CallSpec{
			Procedure: "/test.v1.EchoService/Echo",
			Transport: procframe.TransportCLI,
			Shape:     procframe.CallShapeUnary,
		},
		&procframe.Request[testRequest]{
			Msg:  &testRequest{Message: "hello"},
			Meta: procframe.Meta{Procedure: "/test.v1.EchoService/Echo"},
		},
		func(context.Context, *procframe.Request[testRequest]) (*procframe.Response[testResponse], error) {
			return nil, nil
		},
		procframe.InterceptorFunc(func(_ procframe.HandlerFunc) procframe.HandlerFunc {
			return func(_ context.Context, conn procframe.Conn) error {
				return conn.Send(procframe.NewAnyResponse(&testRequest{Message: "wrong"}))
			}
		}),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	var statusErr *procframe.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("want StatusError, got %T: %v", err, err)
	}
	if statusErr.Code() != procframe.CodeInternal {
		t.Fatalf("want internal, got %s", statusErr.Code())
	}
}

func TestInvokeUnary_HandlerError(t *testing.T) {
	t.Parallel()

	_, err := procframe.InvokeUnary(
		t.Context(),
		procframe.CallSpec{
			Procedure: "/test.v1.EchoService/Echo",
			Transport: procframe.TransportCLI,
			Shape:     procframe.CallShapeUnary,
		},
		&procframe.Request[testRequest]{Msg: &testRequest{Message: "hello"}},
		func(context.Context, *procframe.Request[testRequest]) (*procframe.Response[testResponse], error) {
			return nil, procframe.NewError(procframe.CodeNotFound, "not found")
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	code, ok := procframe.CodeOf(err)
	if !ok || code != procframe.CodeNotFound {
		t.Fatalf("want not_found, got %v", err)
	}
}

func TestInvokeServerStream_NoInterceptor(t *testing.T) {
	t.Parallel()

	stream := &collectingStream{getCtx: t.Context}
	err := procframe.InvokeServerStream(
		t.Context(),
		procframe.CallSpec{
			Procedure: "/test.v1.TickService/Watch",
			Transport: procframe.TransportWS,
			Shape:     procframe.CallShapeServerStream,
		},
		&procframe.Request[testRequest]{Msg: &testRequest{Message: "ping"}},
		stream,
		func(_ context.Context, req *procframe.Request[testRequest], stream procframe.ServerStream[testResponse]) error {
			return stream.Send(&procframe.Response[testResponse]{
				Msg: &testResponse{Message: "pong:" + req.Msg.Message},
			})
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stream.sent) != 1 || stream.sent[0].Msg.Message != "pong:ping" {
		t.Fatalf("unexpected stream output: %v", stream.sent)
	}
}

func TestInvokeServerStream_ConnSendWrapping(t *testing.T) {
	t.Parallel()

	spec := procframe.CallSpec{
		Procedure: "/test.v1.TickService/Watch",
		Transport: procframe.TransportWS,
		Shape:     procframe.CallShapeServerStream,
	}
	stream := &collectingStream{getCtx: t.Context}
	var events []string

	err := procframe.InvokeServerStream(
		t.Context(),
		spec,
		&procframe.Request[testRequest]{
			Msg:  &testRequest{Message: "ping"},
			Meta: procframe.Meta{Procedure: spec.Procedure},
		},
		stream,
		func(_ context.Context, req *procframe.Request[testRequest], stream procframe.ServerStream[testResponse]) error {
			events = append(events, "handler before send")
			if req.Msg.Message != "wrapped:ping" {
				t.Fatalf("want wrapped request, got %q", req.Msg.Message)
			}
			if err := stream.Send(&procframe.Response[testResponse]{
				Msg: &testResponse{Message: "one"},
			}); err != nil {
				return err
			}
			events = append(events, "handler after send")
			return nil
		},
		// Interceptor that wraps both Receive and Send on the Conn
		procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
			return func(ctx context.Context, conn procframe.Conn) error {
				events = append(events, "outer before")
				err := next(ctx, &modifyReceiveAndSendConn{
					Conn: conn,
					modifyRecv: func(req procframe.AnyRequest) {
						msg, ok := req.Any().(*testRequest)
						if !ok {
							t.Fatalf("want *testRequest, got %T", req.Any())
						}
						msg.Message = "wrapped:" + msg.Message
					},
					modifySend: func(resp procframe.AnyResponse) {
						events = append(events, "send intercepted")
						msg, ok := resp.Any().(*testResponse)
						if !ok {
							t.Fatalf("want *testResponse, got %T", resp.Any())
						}
						msg.Message = "wrapped:" + msg.Message
					},
				})
				events = append(events, "outer after")
				return err
			}
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stream.sent) != 1 {
		t.Fatalf("want 1 response, got %d", len(stream.sent))
	}
	if got := stream.sent[0].Msg.Message; got != "wrapped:one" {
		t.Fatalf("want wrapped send, got %q", got)
	}

	want := []string{
		"outer before",
		"handler before send",
		"send intercepted",
		"handler after send",
		"outer after",
	}
	if !slices.Equal(events, want) {
		t.Fatalf("want %v, got %v", want, events)
	}
}

func TestInvokeClientStream_NoInterceptor(t *testing.T) {
	t.Parallel()

	stream := &sliceClientStream{
		getCtx: t.Context,
		msgs:   []*testRequest{{Message: "a"}, {Message: "b"}, {Message: "c"}},
	}
	resp, err := procframe.InvokeClientStream(
		t.Context(),
		procframe.CallSpec{
			Procedure: "/test.v1.Svc/Collect",
			Transport: procframe.TransportConnect,
			Shape:     procframe.CallShapeClientStream,
		},
		stream,
		func(_ context.Context, cs procframe.ClientStream[testRequest]) (*procframe.Response[testResponse], error) {
			var msgs []string
			for {
				req, err := cs.Receive()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return nil, err
				}
				msgs = append(msgs, req.Msg.Message)
			}
			return &procframe.Response[testResponse]{
				Msg: &testResponse{Message: "collected:" + strings.Join(msgs, ",")},
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Message != "collected:a,b,c" {
		t.Fatalf("want collected:a,b,c, got %q", resp.Msg.Message)
	}
}

func TestInvokeClientStream_WithInterceptor(t *testing.T) {
	t.Parallel()

	stream := &sliceClientStream{
		getCtx: t.Context,
		msgs:   []*testRequest{{Message: "x"}},
	}
	var events []string

	resp, err := procframe.InvokeClientStream(
		t.Context(),
		procframe.CallSpec{
			Procedure: "/test.v1.Svc/Collect",
			Transport: procframe.TransportConnect,
			Shape:     procframe.CallShapeClientStream,
		},
		stream,
		func(_ context.Context, cs procframe.ClientStream[testRequest]) (*procframe.Response[testResponse], error) {
			events = append(events, "handler")
			req, err := cs.Receive()
			if err != nil {
				return nil, err
			}
			return &procframe.Response[testResponse]{
				Msg: &testResponse{Message: "got:" + req.Msg.Message},
			}, nil
		},
		procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
			return func(ctx context.Context, conn procframe.Conn) error {
				events = append(events, "interceptor before")
				// Interceptor sees the same generic Conn regardless of shape
				if conn.Spec().Shape != procframe.CallShapeClientStream {
					t.Fatalf("want client_stream shape, got %q", conn.Spec().Shape)
				}
				err := next(ctx, conn)
				events = append(events, "interceptor after")
				return err
			}
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Message != "got:x" {
		t.Fatalf("want got:x, got %q", resp.Msg.Message)
	}

	want := []string{"interceptor before", "handler", "interceptor after"}
	if !slices.Equal(events, want) {
		t.Fatalf("want %v, got %v", want, events)
	}
}

func TestInvokeBidi_NoInterceptor(t *testing.T) {
	t.Parallel()

	stream := &sliceBidiStream{
		getCtx: t.Context,
		msgs:   []*testRequest{{Message: "a"}, {Message: "b"}},
	}

	err := procframe.InvokeBidi(
		t.Context(),
		procframe.CallSpec{
			Procedure: "/test.v1.Svc/Chat",
			Transport: procframe.TransportWS,
			Shape:     procframe.CallShapeBidi,
		},
		stream,
		func(_ context.Context, bs procframe.BidiStream[testRequest, testResponse]) error {
			for {
				req, err := bs.Receive()
				if errors.Is(err, io.EOF) {
					return nil
				}
				if err != nil {
					return err
				}
				if err := bs.Send(&procframe.Response[testResponse]{
					Msg: &testResponse{Message: "echo:" + req.Msg.Message},
				}); err != nil {
					return err
				}
			}
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stream.sent) != 2 {
		t.Fatalf("want 2 responses, got %d", len(stream.sent))
	}
	if stream.sent[0].Msg.Message != "echo:a" || stream.sent[1].Msg.Message != "echo:b" {
		t.Fatalf("unexpected responses: %v, %v", stream.sent[0].Msg.Message, stream.sent[1].Msg.Message)
	}
}

func TestInvokeBidi_WithInterceptor(t *testing.T) {
	t.Parallel()

	stream := &sliceBidiStream{
		getCtx: t.Context,
		msgs:   []*testRequest{{Message: "hi"}},
	}
	var events []string

	err := procframe.InvokeBidi(
		t.Context(),
		procframe.CallSpec{
			Procedure: "/test.v1.Svc/Chat",
			Transport: procframe.TransportWS,
			Shape:     procframe.CallShapeBidi,
		},
		stream,
		func(_ context.Context, bs procframe.BidiStream[testRequest, testResponse]) error {
			events = append(events, "handler")
			req, err := bs.Receive()
			if err != nil {
				return err
			}
			return bs.Send(&procframe.Response[testResponse]{
				Msg: &testResponse{Message: "reply:" + req.Msg.Message},
			})
		},
		// Interceptor wraps Send via conn decorator
		procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
			return func(ctx context.Context, conn procframe.Conn) error {
				events = append(events, "interceptor before")
				err := next(ctx, &modifySendConn{
					Conn: conn,
					modify: func(resp procframe.AnyResponse) {
						msg, ok := resp.Any().(*testResponse)
						if !ok {
							t.Fatalf("want *testResponse, got %T", resp.Any())
						}
						msg.Message = "modified:" + msg.Message
					},
				})
				events = append(events, "interceptor after")
				return err
			}
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stream.sent) != 1 || stream.sent[0].Msg.Message != "modified:reply:hi" {
		t.Fatalf("unexpected response: %v", stream.sent)
	}

	want := []string{"interceptor before", "handler", "interceptor after"}
	if !slices.Equal(events, want) {
		t.Fatalf("want %v, got %v", want, events)
	}
}

func TestInvokeServerStream_HandlerError(t *testing.T) {
	t.Parallel()

	stream := &collectingStream{getCtx: t.Context}
	err := procframe.InvokeServerStream(
		t.Context(),
		procframe.CallSpec{
			Procedure: "/test.v1.TickService/Watch",
			Transport: procframe.TransportCLI,
			Shape:     procframe.CallShapeServerStream,
		},
		&procframe.Request[testRequest]{Msg: &testRequest{Message: "ping"}},
		stream,
		func(context.Context, *procframe.Request[testRequest], procframe.ServerStream[testResponse]) error {
			return procframe.NewError(procframe.CodeInternal, "boom")
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	code, ok := procframe.CodeOf(err)
	if !ok || code != procframe.CodeInternal {
		t.Fatalf("want internal, got %v", err)
	}
	if len(stream.sent) != 0 {
		t.Fatalf("want 0 responses sent, got %d", len(stream.sent))
	}
}

func TestInvokeUnary_NilHandlerResponse(t *testing.T) {
	t.Parallel()

	resp, err := procframe.InvokeUnary(
		t.Context(),
		procframe.CallSpec{
			Procedure: "/test.v1.EchoService/Echo",
			Transport: procframe.TransportCLI,
			Shape:     procframe.CallShapeUnary,
		},
		&procframe.Request[testRequest]{Msg: &testRequest{Message: "hello"}},
		func(context.Context, *procframe.Request[testRequest]) (*procframe.Response[testResponse], error) {
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Fatalf("want nil response, got %v", resp)
	}
}

func TestInvokeUnary_NilHandlerPanics(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			// Panic is acceptable for nil handler — it's a programming error.
			t.Logf("recovered panic for nil handler: %v", r)
		}
	}()

	_, err := procframe.InvokeUnary[string, string](
		t.Context(),
		procframe.CallSpec{Procedure: "/test/Nil", Transport: procframe.TransportCLI, Shape: procframe.CallShapeUnary},
		&procframe.Request[string]{Msg: ptrTo("hello")},
		nil,
	)
	if err == nil {
		t.Fatal("expected error or panic for nil handler")
	}
}

func TestInvokeUnary_NilRequest(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Logf("recovered panic for nil request: %v", r)
		}
	}()

	_, err := procframe.InvokeUnary(
		t.Context(),
		procframe.CallSpec{Procedure: "/test/Nil", Transport: procframe.TransportCLI, Shape: procframe.CallShapeUnary},
		(*procframe.Request[string])(nil),
		func(_ context.Context, _ *procframe.Request[string]) (*procframe.Response[string], error) {
			return &procframe.Response[string]{Msg: ptrTo("ok")}, nil
		},
	)
	// Either error or panic is acceptable.
	_ = err
}

func TestInterceptorFunc_Nil(t *testing.T) {
	t.Parallel()

	var nilFunc procframe.InterceptorFunc
	called := false
	resp, err := procframe.InvokeUnary(
		t.Context(),
		procframe.CallSpec{
			Procedure: "/test/NilIF",
			Transport: procframe.TransportCLI,
			Shape:     procframe.CallShapeUnary,
		},
		&procframe.Request[string]{Msg: ptrTo("hello")},
		func(_ context.Context, _ *procframe.Request[string]) (*procframe.Response[string], error) {
			called = true
			return &procframe.Response[string]{Msg: ptrTo("ok")}, nil
		},
		nilFunc,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not called through nil InterceptorFunc")
	}
	if resp == nil || resp.Msg == nil || *resp.Msg != "ok" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestUnaryConn_DoubleReceive(t *testing.T) {
	t.Parallel()

	var firstReceiveOK bool
	var secondReceiveErr error

	_, err := procframe.InvokeUnary(
		t.Context(),
		procframe.CallSpec{
			Procedure: "/test/DoubleRecv",
			Transport: procframe.TransportCLI,
			Shape:     procframe.CallShapeUnary,
		},
		&procframe.Request[string]{Msg: ptrTo("hello")},
		func(_ context.Context, _ *procframe.Request[string]) (*procframe.Response[string], error) {
			return &procframe.Response[string]{Msg: ptrTo("ok")}, nil
		},
		procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
			return func(ctx context.Context, conn procframe.Conn) error {
				// First Receive should succeed.
				_, err := conn.Receive()
				firstReceiveOK = err == nil

				// Second Receive should return io.EOF.
				_, secondReceiveErr = conn.Receive()

				return next(ctx, conn)
			}
		}),
	)
	// The handler might fail because the interceptor consumed the request.
	_ = err

	if !firstReceiveOK {
		t.Fatal("first Receive should succeed")
	}
	if !errors.Is(secondReceiveErr, io.EOF) {
		t.Fatalf("second Receive should return io.EOF, got: %v", secondReceiveErr)
	}
}

func TestServerStreamConn_DoubleReceive(t *testing.T) {
	t.Parallel()

	var firstReceiveOK bool
	var secondReceiveErr error

	fakeStream := &fakeServerStream[string]{t: t}

	err := procframe.InvokeServerStream(
		t.Context(),
		procframe.CallSpec{
			Procedure: "/test/SSDoubleRecv",
			Transport: procframe.TransportCLI,
			Shape:     procframe.CallShapeServerStream,
		},
		&procframe.Request[string]{Msg: ptrTo("hello")},
		fakeStream,
		func(_ context.Context, _ *procframe.Request[string], _ procframe.ServerStream[string]) error {
			return nil
		},
		procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
			return func(ctx context.Context, conn procframe.Conn) error {
				_, err := conn.Receive()
				firstReceiveOK = err == nil

				_, secondReceiveErr = conn.Receive()

				return next(ctx, conn)
			}
		}),
	)
	_ = err

	if !firstReceiveOK {
		t.Fatal("first Receive should succeed")
	}
	if !errors.Is(secondReceiveErr, io.EOF) {
		t.Fatalf("second Receive should return io.EOF, got: %v", secondReceiveErr)
	}
}

func TestNewAnyResponseWithMeta(t *testing.T) {
	t.Parallel()

	msg := &testResponse{Message: "hello"}
	meta := procframe.Meta{
		Procedure: "/test.v1.Svc/Foo",
		RequestID: "req-123",
	}

	resp := procframe.NewAnyResponseWithMeta(msg, meta)

	if got, ok := resp.Any().(*testResponse); !ok || got.Message != "hello" {
		t.Fatalf("want *testResponse{hello}, got %T %v", resp.Any(), resp.Any())
	}
	if resp.Meta() == nil {
		t.Fatal("want non-nil Meta")
	}
	if resp.Meta().Procedure != "/test.v1.Svc/Foo" {
		t.Fatalf("want procedure /test.v1.Svc/Foo, got %q", resp.Meta().Procedure)
	}
	if resp.Meta().RequestID != "req-123" {
		t.Fatalf("want request ID req-123, got %q", resp.Meta().RequestID)
	}
}

type ctxKey string

func TestInterceptorAccessesConnContext(t *testing.T) {
	t.Parallel()

	marker := ctxKey("marker")

	t.Run("Unary", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(t.Context(), marker, "unary-val")
		spec := procframe.CallSpec{
			Procedure: "/test/Ctx",
			Transport: procframe.TransportCLI,
			Shape:     procframe.CallShapeUnary,
		}

		_, err := procframe.InvokeUnary(
			ctx, spec,
			&procframe.Request[testRequest]{Msg: &testRequest{Message: "hi"}},
			func(_ context.Context, _ *procframe.Request[testRequest]) (*procframe.Response[testResponse], error) {
				return &procframe.Response[testResponse]{Msg: &testResponse{Message: "ok"}}, nil
			},
			procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
				return func(ctx context.Context, conn procframe.Conn) error {
					v, ok := conn.Context().Value(marker).(string)
					if !ok || v != "unary-val" {
						t.Fatalf("want unary-val in conn context, got %q", v)
					}
					return next(ctx, conn)
				}
			}),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("ClientStream", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(t.Context(), marker, "cs-val")
		stream := &sliceClientStream{
			getCtx: func() context.Context { return ctx },
			msgs:   []*testRequest{{Message: "a"}},
		}

		_, err := procframe.InvokeClientStream(
			ctx,
			procframe.CallSpec{
				Procedure: "/test/Ctx",
				Transport: procframe.TransportCLI,
				Shape:     procframe.CallShapeClientStream,
			},
			stream,
			func(_ context.Context, cs procframe.ClientStream[testRequest]) (*procframe.Response[testResponse], error) {
				v, ok := cs.Context().Value(marker).(string)
				if !ok || v != "cs-val" {
					t.Fatalf("want cs-val in stream context, got %q", v)
				}
				for {
					_, err := cs.Receive()
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						return nil, err
					}
				}
				return &procframe.Response[testResponse]{Msg: &testResponse{Message: "ok"}}, nil
			},
			procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
				return func(ctx context.Context, conn procframe.Conn) error {
					v, ok := conn.Context().Value(marker).(string)
					if !ok || v != "cs-val" {
						t.Fatalf("want cs-val in conn context, got %q", v)
					}
					return next(ctx, conn)
				}
			}),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("ServerStream", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(t.Context(), marker, "ss-val")
		stream := &collectingStream{
			getCtx: func() context.Context { return ctx },
		}

		err := procframe.InvokeServerStream(
			ctx,
			procframe.CallSpec{
				Procedure: "/test/Ctx",
				Transport: procframe.TransportCLI,
				Shape:     procframe.CallShapeServerStream,
			},
			&procframe.Request[testRequest]{Msg: &testRequest{Message: "hi"}},
			stream,
			func(_ context.Context, _ *procframe.Request[testRequest], ss procframe.ServerStream[testResponse]) error {
				v, ok := ss.Context().Value(marker).(string)
				if !ok || v != "ss-val" {
					t.Fatalf("want ss-val in stream context, got %q", v)
				}
				return nil
			},
			procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
				return func(ctx context.Context, conn procframe.Conn) error {
					v, ok := conn.Context().Value(marker).(string)
					if !ok || v != "ss-val" {
						t.Fatalf("want ss-val in conn context, got %q", v)
					}
					return next(ctx, conn)
				}
			}),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Bidi", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(t.Context(), marker, "bidi-val")
		stream := &sliceBidiStream{
			getCtx: func() context.Context { return ctx },
			msgs:   []*testRequest{{Message: "a"}},
		}

		err := procframe.InvokeBidi(
			ctx,
			procframe.CallSpec{
				Procedure: "/test/Ctx",
				Transport: procframe.TransportCLI,
				Shape:     procframe.CallShapeBidi,
			},
			stream,
			func(_ context.Context, bs procframe.BidiStream[testRequest, testResponse]) error {
				v, ok := bs.Context().Value(marker).(string)
				if !ok || v != "bidi-val" {
					t.Fatalf("want bidi-val in stream context, got %q", v)
				}
				for {
					_, err := bs.Receive()
					if errors.Is(err, io.EOF) {
						return nil
					}
					if err != nil {
						return err
					}
				}
			},
			procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
				return func(ctx context.Context, conn procframe.Conn) error {
					v, ok := conn.Context().Value(marker).(string)
					if !ok || v != "bidi-val" {
						t.Fatalf("want bidi-val in conn context, got %q", v)
					}
					if conn.Spec().Shape != procframe.CallShapeBidi {
						t.Fatalf("want shape bidi, got %q", conn.Spec().Shape)
					}
					return next(ctx, conn)
				}
			}),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestInterceptorAccessesRequestMetaAndSpec(t *testing.T) {
	t.Parallel()

	t.Run("Unary", func(t *testing.T) {
		t.Parallel()
		spec := procframe.CallSpec{
			Procedure: "/test.v1.Svc/MetaSpec",
			Transport: procframe.TransportConnect,
			Shape:     procframe.CallShapeUnary,
		}
		req := &procframe.Request[testRequest]{
			Msg:  &testRequest{Message: "hi"},
			Meta: procframe.Meta{Procedure: spec.Procedure, RequestID: "rid-1"},
		}

		_, err := procframe.InvokeUnary(
			t.Context(), spec, req,
			func(_ context.Context, _ *procframe.Request[testRequest]) (*procframe.Response[testResponse], error) {
				return &procframe.Response[testResponse]{Msg: &testResponse{Message: "ok"}}, nil
			},
			procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
				return func(ctx context.Context, conn procframe.Conn) error {
					// Wrap Receive to inspect the request without consuming it from inner handler's perspective.
					return next(ctx, &modifyReceiveConn{
						Conn: conn,
						modify: func(anyReq procframe.AnyRequest) {
							if anyReq.Meta() == nil {
								t.Fatal("want non-nil Meta from AnyRequest")
							}
							if anyReq.Meta().Procedure != spec.Procedure {
								t.Fatalf("want procedure %q, got %q", spec.Procedure, anyReq.Meta().Procedure)
							}
							if anyReq.Meta().RequestID != "rid-1" {
								t.Fatalf("want request ID rid-1, got %q", anyReq.Meta().RequestID)
							}
							if anyReq.Spec().Transport != procframe.TransportConnect {
								t.Fatalf("want transport connect, got %q", anyReq.Spec().Transport)
							}
							if anyReq.Spec().Shape != procframe.CallShapeUnary {
								t.Fatalf("want shape unary, got %q", anyReq.Spec().Shape)
							}
						},
					})
				}
			}),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("ServerStream", func(t *testing.T) {
		t.Parallel()
		spec := procframe.CallSpec{
			Procedure: "/test.v1.Svc/SSSpec",
			Transport: procframe.TransportWS,
			Shape:     procframe.CallShapeServerStream,
		}

		err := procframe.InvokeServerStream(
			t.Context(), spec,
			&procframe.Request[testRequest]{Msg: &testRequest{Message: "hi"}},
			&collectingStream{getCtx: t.Context},
			func(_ context.Context, _ *procframe.Request[testRequest], _ procframe.ServerStream[testResponse]) error {
				return nil
			},
			procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
				return func(ctx context.Context, conn procframe.Conn) error {
					if conn.Spec().Transport != procframe.TransportWS {
						t.Fatalf("want transport ws, got %q", conn.Spec().Transport)
					}
					if conn.Spec().Shape != procframe.CallShapeServerStream {
						t.Fatalf("want shape server_stream, got %q", conn.Spec().Shape)
					}
					return next(ctx, conn)
				}
			}),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestInterceptorAccessesResponseMeta(t *testing.T) {
	t.Parallel()

	spec := procframe.CallSpec{
		Procedure: "/test.v1.Svc/ResMeta",
		Transport: procframe.TransportCLI,
		Shape:     procframe.CallShapeUnary,
	}

	var capturedMeta *procframe.Meta

	_, err := procframe.InvokeUnary(
		t.Context(), spec,
		&procframe.Request[testRequest]{Msg: &testRequest{Message: "hi"}},
		func(_ context.Context, _ *procframe.Request[testRequest]) (*procframe.Response[testResponse], error) {
			return &procframe.Response[testResponse]{
				Msg:  &testResponse{Message: "ok"},
				Meta: procframe.Meta{Procedure: spec.Procedure, RequestID: "resp-rid"},
			}, nil
		},
		procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
			return func(ctx context.Context, conn procframe.Conn) error {
				return next(ctx, &modifySendConn{
					Conn: conn,
					modify: func(resp procframe.AnyResponse) {
						capturedMeta = resp.Meta()
					},
				})
			}
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedMeta == nil {
		t.Fatal("want non-nil Meta from AnyResponse")
	}
	if capturedMeta.Procedure != spec.Procedure {
		t.Fatalf("want procedure %q, got %q", spec.Procedure, capturedMeta.Procedure)
	}
	if capturedMeta.RequestID != "resp-rid" {
		t.Fatalf("want request ID resp-rid, got %q", capturedMeta.RequestID)
	}
}

func TestCastRequest_InterceptorReplacesRequest(t *testing.T) {
	t.Parallel()

	spec := procframe.CallSpec{
		Procedure: "/test.v1.Svc/Cast",
		Transport: procframe.TransportCLI,
		Shape:     procframe.CallShapeUnary,
	}

	t.Run("happy", func(t *testing.T) {
		t.Parallel()
		customMeta := procframe.Meta{Procedure: spec.Procedure, RequestID: "custom-rid"}

		var handlerMsg string
		var handlerMeta procframe.Meta

		_, err := procframe.InvokeUnary(
			t.Context(), spec,
			&procframe.Request[testRequest]{Msg: &testRequest{Message: "original"}},
			func(_ context.Context, req *procframe.Request[testRequest]) (*procframe.Response[testResponse], error) {
				handlerMsg = req.Msg.Message
				handlerMeta = req.Meta
				return &procframe.Response[testResponse]{Msg: &testResponse{Message: "ok"}}, nil
			},
			procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
				return func(ctx context.Context, conn procframe.Conn) error {
					return next(ctx, &replaceReceiveConn{
						Conn: conn,
						recv: func() (procframe.AnyRequest, error) {
							return &fakeAnyRequest{
								any:  &testRequest{Message: "replaced"},
								meta: &customMeta,
								spec: spec,
							}, nil
						},
					})
				}
			}),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if handlerMsg != "replaced" {
			t.Fatalf("want replaced, got %q", handlerMsg)
		}
		if handlerMeta.RequestID != "custom-rid" {
			t.Fatalf("want custom-rid, got %q", handlerMeta.RequestID)
		}
	})

	t.Run("nil_meta", func(t *testing.T) {
		t.Parallel()

		var handlerMeta procframe.Meta

		_, err := procframe.InvokeUnary(
			t.Context(), spec,
			&procframe.Request[testRequest]{Msg: &testRequest{Message: "original"}},
			func(_ context.Context, req *procframe.Request[testRequest]) (*procframe.Response[testResponse], error) {
				handlerMeta = req.Meta
				return &procframe.Response[testResponse]{Msg: &testResponse{Message: "ok"}}, nil
			},
			procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
				return func(ctx context.Context, conn procframe.Conn) error {
					return next(ctx, &replaceReceiveConn{
						Conn: conn,
						recv: func() (procframe.AnyRequest, error) {
							return &fakeAnyRequest{
								any:  &testRequest{Message: "replaced"},
								meta: nil,
								spec: spec,
							}, nil
						},
					})
				}
			}),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if handlerMeta.Procedure != "" || handlerMeta.RequestID != "" || handlerMeta.SessionID != "" ||
			handlerMeta.Labels != nil {
			t.Fatalf("want zero Meta, got %+v", handlerMeta)
		}
	})

	t.Run("type_mismatch", func(t *testing.T) {
		t.Parallel()

		_, err := procframe.InvokeUnary(
			t.Context(), spec,
			&procframe.Request[testRequest]{Msg: &testRequest{Message: "original"}},
			func(_ context.Context, _ *procframe.Request[testRequest]) (*procframe.Response[testResponse], error) {
				t.Fatal("handler must not run")
				return nil, nil
			},
			procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
				return func(ctx context.Context, conn procframe.Conn) error {
					return next(ctx, &replaceReceiveConn{
						Conn: conn,
						recv: func() (procframe.AnyRequest, error) {
							return &fakeAnyRequest{
								any:  ptrTo("wrong type"),
								spec: spec,
							}, nil
						},
					})
				}
			}),
		)
		if err == nil {
			t.Fatal("expected error for type mismatch")
		}
		code, ok := procframe.CodeOf(err)
		if !ok || code != procframe.CodeInternal {
			t.Fatalf("want internal, got %v", err)
		}
		if !strings.Contains(err.Error(), "unexpected request type") {
			t.Fatalf("want 'unexpected request type' in error, got %q", err.Error())
		}
	})

	t.Run("nil_request", func(t *testing.T) {
		t.Parallel()

		_, err := procframe.InvokeUnary(
			t.Context(), spec,
			&procframe.Request[testRequest]{Msg: &testRequest{Message: "original"}},
			func(_ context.Context, _ *procframe.Request[testRequest]) (*procframe.Response[testResponse], error) {
				t.Fatal("handler must not run")
				return nil, nil
			},
			procframe.InterceptorFunc(func(next procframe.HandlerFunc) procframe.HandlerFunc {
				return func(ctx context.Context, conn procframe.Conn) error {
					return next(ctx, &replaceReceiveConn{
						Conn: conn,
						recv: func() (procframe.AnyRequest, error) {
							return nil, nil
						},
					})
				}
			}),
		)
		if err == nil {
			t.Fatal("expected error for nil request")
		}
		code, ok := procframe.CodeOf(err)
		if !ok || code != procframe.CodeInternal {
			t.Fatalf("want internal, got %v", err)
		}
		if !strings.Contains(err.Error(), "nil request") {
			t.Fatalf("want 'nil request' in error, got %q", err.Error())
		}
	})
}

func TestInvokeClientStream_HandlerErrorAndNilResponse(t *testing.T) {
	t.Parallel()

	spec := procframe.CallSpec{
		Procedure: "/test.v1.Svc/CSErr",
		Transport: procframe.TransportConnect,
		Shape:     procframe.CallShapeClientStream,
	}

	t.Run("handler_error", func(t *testing.T) {
		t.Parallel()
		stream := &sliceClientStream{
			getCtx: t.Context,
			msgs:   []*testRequest{{Message: "a"}},
		}

		_, err := procframe.InvokeClientStream(
			t.Context(), spec, stream,
			func(_ context.Context, _ procframe.ClientStream[testRequest]) (*procframe.Response[testResponse], error) {
				return nil, procframe.NewError(procframe.CodeNotFound, "not found")
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
		code, ok := procframe.CodeOf(err)
		if !ok || code != procframe.CodeNotFound {
			t.Fatalf("want not_found, got %v", err)
		}
	})

	t.Run("nil_response", func(t *testing.T) {
		t.Parallel()
		stream := &sliceClientStream{
			getCtx: t.Context,
			msgs:   []*testRequest{{Message: "a"}},
		}

		resp, err := procframe.InvokeClientStream(
			t.Context(), spec, stream,
			func(_ context.Context, _ procframe.ClientStream[testRequest]) (*procframe.Response[testResponse], error) {
				return nil, nil
			},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp != nil {
			t.Fatalf("want nil response, got %+v", resp)
		}
	})
}

// --- helpers ---

func ptrTo[T any](v T) *T { return &v }

// fakeAnyRequest implements procframe.AnyRequest with configurable returns.
type fakeAnyRequest struct {
	any  any
	meta *procframe.Meta
	spec procframe.CallSpec
}

func (f *fakeAnyRequest) Any() any                 { return f.any }
func (f *fakeAnyRequest) Meta() *procframe.Meta    { return f.meta }
func (f *fakeAnyRequest) Spec() procframe.CallSpec { return f.spec }

// replaceReceiveConn overrides Receive() on a wrapped Conn.
type replaceReceiveConn struct {
	procframe.Conn
	recv func() (procframe.AnyRequest, error)
}

func (c *replaceReceiveConn) Receive() (procframe.AnyRequest, error) {
	return c.recv()
}

type fakeServerStream[T any] struct {
	t *testing.T
}

func (s *fakeServerStream[T]) Context() context.Context            { return s.t.Context() }
func (s *fakeServerStream[T]) Send(_ *procframe.Response[T]) error { return nil }

// --- Conn decorator helpers for tests ---

type modifyReceiveConn struct {
	procframe.Conn
	modify func(procframe.AnyRequest)
}

func (c *modifyReceiveConn) Receive() (procframe.AnyRequest, error) {
	req, err := c.Conn.Receive()
	if err != nil {
		return nil, err
	}
	c.modify(req)
	return req, nil
}

type modifySendConn struct {
	procframe.Conn
	modify func(procframe.AnyResponse)
}

func (c *modifySendConn) Send(resp procframe.AnyResponse) error {
	c.modify(resp)
	return c.Conn.Send(resp)
}

type modifyReceiveAndSendConn struct {
	procframe.Conn
	modifyRecv func(procframe.AnyRequest)
	modifySend func(procframe.AnyResponse)
}

func (c *modifyReceiveAndSendConn) Receive() (procframe.AnyRequest, error) {
	req, err := c.Conn.Receive()
	if err != nil {
		return nil, err
	}
	c.modifyRecv(req)
	return req, nil
}

func (c *modifyReceiveAndSendConn) Send(resp procframe.AnyResponse) error {
	c.modifySend(resp)
	return c.Conn.Send(resp)
}

// TestInvokeUnary_InterceptorWrongTypeSend verifies that an interceptor
// sending a response of the wrong type produces a clean error, not a panic.
func TestInvokeUnary_InterceptorWrongTypeSend(t *testing.T) {
	t.Parallel()

	// An interceptor that sends an AnyResponse with wrong underlying type.
	badInterceptor := procframe.InterceptorFunc(func(_ procframe.HandlerFunc) procframe.HandlerFunc {
		return func(_ context.Context, conn procframe.Conn) error {
			// Skip Receive, directly send a response with wrong type.
			return conn.Send(procframe.NewAnyResponse("wrong-type-string"))
		}
	})

	handler := func(_ context.Context, _ *procframe.Request[int]) (*procframe.Response[int], error) {
		return &procframe.Response[int]{Msg: new(int)}, nil
	}

	_, err := procframe.InvokeUnary(
		t.Context(),
		procframe.CallSpec{Procedure: "/test", Transport: "test", Shape: procframe.CallShapeUnary},
		&procframe.Request[int]{Msg: new(int)},
		handler,
		badInterceptor,
	)
	if err == nil {
		t.Fatal("expected error for wrong response type from interceptor")
	}
	// Verify it's a clean error, not a panic.
	checkNoInternalLeak(t, err.Error())
}
