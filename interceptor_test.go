package procframe_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/shuymn/procframe"
)

type testRequest struct {
	Message string
}

type testResponse struct {
	Message string
}

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

func TestInvokeUnaryInterceptors(t *testing.T) {
	t.Parallel()

	spec := procframe.CallSpec{
		Procedure:  "/test.v1.EchoService/Echo",
		Transport:  procframe.TransportCLI,
		StreamType: procframe.StreamTypeUnary,
	}
	req := &procframe.Request[testRequest]{
		Msg: &testRequest{Message: "hello"},
		Meta: procframe.Meta{
			Procedure: spec.Procedure,
		},
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
		procframe.UnaryInterceptorFunc(func(next procframe.UnaryFunc) procframe.UnaryFunc {
			return func(ctx context.Context, req procframe.AnyRequest) (procframe.AnyResponse, error) {
				events = append(events, "outer before")
				if req.Spec() != spec {
					t.Fatalf("unexpected spec: %+v", req.Spec())
				}
				msg, ok := req.Any().(*testRequest)
				if !ok {
					t.Fatalf("want *testRequest, got %T", req.Any())
				}
				msg.Message = "wrapped:" + msg.Message
				resp, err := next(ctx, req)
				events = append(events, "outer after")
				return resp, err
			}
		}),
		procframe.UnaryInterceptorFunc(func(next procframe.UnaryFunc) procframe.UnaryFunc {
			return func(ctx context.Context, req procframe.AnyRequest) (procframe.AnyResponse, error) {
				events = append(events, "inner before")
				resp, err := next(ctx, req)
				events = append(events, "inner after")
				return resp, err
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

func TestInvokeUnaryInterceptorShortCircuit(t *testing.T) {
	t.Parallel()

	spec := procframe.CallSpec{
		Procedure:  "/test.v1.EchoService/Echo",
		Transport:  procframe.TransportConnect,
		StreamType: procframe.StreamTypeUnary,
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
		procframe.UnaryInterceptorFunc(func(_ procframe.UnaryFunc) procframe.UnaryFunc {
			return func(context.Context, procframe.AnyRequest) (procframe.AnyResponse, error) {
				return procframe.NewAnyResponse(&testResponse{Message: "short-circuit"}), nil
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

func TestInvokeServerStreamInterceptors(t *testing.T) {
	t.Parallel()

	spec := procframe.CallSpec{
		Procedure:  "/test.v1.TickService/Watch",
		Transport:  procframe.TransportWS,
		StreamType: procframe.StreamTypeServerStream,
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
		procframe.ServerStreamInterceptorFunc(func(next procframe.ServerStreamFunc) procframe.ServerStreamFunc {
			return func(ctx context.Context, req procframe.AnyRequest, stream procframe.AnyServerStream) error {
				events = append(events, "outer before")
				msg, ok := req.Any().(*testRequest)
				if !ok {
					t.Fatalf("want *testRequest, got %T", req.Any())
				}
				msg.Message = "wrapped:" + msg.Message
				err := next(ctx, req, stream)
				events = append(events, "outer after")
				return err
			}
		}),
		procframe.StreamSendInterceptorFunc(func(next procframe.StreamSendFunc) procframe.StreamSendFunc {
			return func(resp procframe.AnyResponse) error {
				events = append(events, "send before")
				msg, ok := resp.Any().(*testResponse)
				if !ok {
					t.Fatalf("want *testResponse, got %T", resp.Any())
				}
				msg.Message = "wrapped:" + msg.Message
				err := next(resp)
				events = append(events, "send after")
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
		"send before",
		"send after",
		"handler after send",
		"outer after",
	}
	if !slices.Equal(events, want) {
		t.Fatalf("want %v, got %v", want, events)
	}
}

func TestInvokeUnaryRejectsUnexpectedResponseType(t *testing.T) {
	t.Parallel()

	_, err := procframe.InvokeUnary(
		t.Context(),
		procframe.CallSpec{
			Procedure:  "/test.v1.EchoService/Echo",
			Transport:  procframe.TransportCLI,
			StreamType: procframe.StreamTypeUnary,
		},
		&procframe.Request[testRequest]{
			Msg:  &testRequest{Message: "hello"},
			Meta: procframe.Meta{Procedure: "/test.v1.EchoService/Echo"},
		},
		func(context.Context, *procframe.Request[testRequest]) (*procframe.Response[testResponse], error) {
			return nil, nil
		},
		procframe.UnaryInterceptorFunc(func(_ procframe.UnaryFunc) procframe.UnaryFunc {
			return func(context.Context, procframe.AnyRequest) (procframe.AnyResponse, error) {
				return procframe.NewAnyResponse(&testRequest{Message: "wrong"}), nil
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
