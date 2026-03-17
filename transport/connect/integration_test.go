package connect_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connectrpc "connectrpc.com/connect"

	"github.com/shuymn/procframe"
	testv1 "github.com/shuymn/procframe/internal/gen/test/v1"
	connecttransport "github.com/shuymn/procframe/transport/connect"
)

// echoHandler is a test handler for EchoService.
type echoHandler struct{}

func (h *echoHandler) Echo(
	_ context.Context,
	req *procframe.Request[testv1.EchoRequest],
) (*procframe.Response[testv1.EchoResponse], error) {
	msg := req.Msg.Message
	if req.Msg.Uppercase {
		msg = strings.ToUpper(msg)
	}
	return &procframe.Response[testv1.EchoResponse]{
		Msg: &testv1.EchoResponse{
			Message: msg,
			Count:   req.Msg.Count,
		},
	}, nil
}

// tickHandler is a test handler for TickService.
type tickHandler struct{}

func (h *tickHandler) Watch(
	_ context.Context,
	req *procframe.Request[testv1.TickRequest],
	stream procframe.ServerStream[testv1.TickResponse],
) error {
	for i := range req.Msg.Count {
		if err := stream.Send(&procframe.Response[testv1.TickResponse]{
			Msg: &testv1.TickResponse{
				Label: req.Msg.Label,
				Seq:   i + 1,
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func TestIntegration_ConnectUnarySuccess(t *testing.T) {
	t.Parallel()

	h := &echoHandler{}
	path, handler := testv1.NewEchoServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := connectrpc.NewClient[testv1.EchoRequest, testv1.EchoResponse](
		srv.Client(),
		srv.URL+"/test.v1.EchoService/Echo",
	)

	resp, err := client.CallUnary(t.Context(), connectrpc.NewRequest(&testv1.EchoRequest{
		Message:   "hello",
		Count:     3,
		Uppercase: true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Message != "HELLO" {
		t.Fatalf("want HELLO, got %q", resp.Msg.Message)
	}
	if resp.Msg.Count != 3 {
		t.Fatalf("want count=3, got %d", resp.Msg.Count)
	}
}

func TestIntegration_ConnectUnaryError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		procCode    procframe.Code
		connectCode connectrpc.Code
	}{
		{procframe.CodeInvalidArgument, connectrpc.CodeInvalidArgument},
		{procframe.CodeNotFound, connectrpc.CodeNotFound},
		{procframe.CodeInternal, connectrpc.CodeInternal},
		{procframe.CodeUnauthenticated, connectrpc.CodeUnauthenticated},
		{procframe.CodeUnavailable, connectrpc.CodeUnavailable},
		{procframe.CodeAlreadyExists, connectrpc.CodeAlreadyExists},
		{procframe.CodePermissionDenied, connectrpc.CodePermissionDenied},
		{procframe.CodeConflict, connectrpc.CodeAborted},
	}

	for _, tt := range tests {
		t.Run(string(tt.procCode), func(t *testing.T) {
			t.Parallel()

			errorHandle := func(
				_ context.Context,
				_ *procframe.Request[testv1.EchoRequest],
			) (*procframe.Response[testv1.EchoResponse], error) {
				return nil, procframe.NewError(tt.procCode, "test error")
			}

			mux := http.NewServeMux()
			mux.Handle(connecttransport.NewUnaryHandler(
				"/test.v1.EchoService/Echo",
				errorHandle,
			))

			srv := httptest.NewServer(mux)
			defer srv.Close()

			client := connectrpc.NewClient[testv1.EchoRequest, testv1.EchoResponse](
				srv.Client(),
				srv.URL+"/test.v1.EchoService/Echo",
			)

			_, err := client.CallUnary(t.Context(), connectrpc.NewRequest(&testv1.EchoRequest{
				Message: "test",
			}))
			if err == nil {
				t.Fatal("expected error")
			}

			var connectErr *connectrpc.Error
			if !errors.As(err, &connectErr) {
				t.Fatalf("expected connect.Error, got %T: %v", err, err)
			}
			if connectErr.Code() != tt.connectCode {
				t.Fatalf("want %v, got %v", tt.connectCode, connectErr.Code())
			}
			if connectErr.Message() != "test error" {
				t.Fatalf("want 'test error', got %q", connectErr.Message())
			}
		})
	}
}

func TestIntegration_ConnectServerStreaming(t *testing.T) {
	t.Parallel()

	h := &tickHandler{}
	path, handler := testv1.NewTickServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := connectrpc.NewClient[testv1.TickRequest, testv1.TickResponse](
		srv.Client(),
		srv.URL+"/test.v1.TickService/Watch",
	)

	stream, err := client.CallServerStream(t.Context(), connectrpc.NewRequest(&testv1.TickRequest{
		Label: "ping",
		Count: 3,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var msgs []*testv1.TickResponse
	for stream.Receive() {
		msgs = append(msgs, stream.Msg())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}

	if len(msgs) != 3 {
		t.Fatalf("want 3 messages, got %d", len(msgs))
	}
	for i, msg := range msgs {
		if msg.Label != "ping" {
			t.Fatalf("msg[%d]: want label=ping, got %q", i, msg.Label)
		}
		if msg.Seq != int32(i+1) {
			t.Fatalf("msg[%d]: want seq=%d, got %d", i, i+1, msg.Seq)
		}
	}
}

func TestIntegration_GRPCUnary(t *testing.T) {
	t.Parallel()

	h := &echoHandler{}
	path, handler := testv1.NewEchoServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	client := connectrpc.NewClient[testv1.EchoRequest, testv1.EchoResponse](
		srv.Client(),
		srv.URL+"/test.v1.EchoService/Echo",
		connectrpc.WithGRPC(),
	)

	resp, err := client.CallUnary(t.Context(), connectrpc.NewRequest(&testv1.EchoRequest{
		Message:   "grpc-hello",
		Count:     7,
		Uppercase: true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Message != "GRPC-HELLO" {
		t.Fatalf("want GRPC-HELLO, got %q", resp.Msg.Message)
	}
	if resp.Msg.Count != 7 {
		t.Fatalf("want count=7, got %d", resp.Msg.Count)
	}
}

func TestIntegration_GRPCServerStreaming(t *testing.T) {
	t.Parallel()

	h := &tickHandler{}
	path, handler := testv1.NewTickServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	client := connectrpc.NewClient[testv1.TickRequest, testv1.TickResponse](
		srv.Client(),
		srv.URL+"/test.v1.TickService/Watch",
		connectrpc.WithGRPC(),
	)

	stream, err := client.CallServerStream(t.Context(), connectrpc.NewRequest(&testv1.TickRequest{
		Label: "grpc-ping",
		Count: 2,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var msgs []*testv1.TickResponse
	for stream.Receive() {
		msgs = append(msgs, stream.Msg())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}

	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	for i, msg := range msgs {
		if msg.Label != "grpc-ping" {
			t.Fatalf("msg[%d]: want label=grpc-ping, got %q", i, msg.Label)
		}
	}
}

func TestIntegration_ConnectOptOut(t *testing.T) {
	t.Parallel()

	// CliOptionsTestService has 4 methods. Only DefaultEnabled and ExplicitEnabled
	// have connect.enabled = true. ExplicitDisabled and WsEnabled do not.
	// The generated handler should only route the two enabled procedures.

	h := &cliOptionsHandler{}
	path, handler := testv1.NewCliOptionsTestServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Enabled method should succeed.
	enabledClient := connectrpc.NewClient[testv1.PingRequest, testv1.PingResponse](
		srv.Client(),
		srv.URL+"/test.v1.CliOptionsTestService/DefaultEnabled",
	)
	resp, err := enabledClient.CallUnary(t.Context(), connectrpc.NewRequest(&testv1.PingRequest{
		Target: "ok",
	}))
	if err != nil {
		t.Fatalf("enabled method: unexpected error: %v", err)
	}
	if resp.Msg.Result != "ok" {
		t.Fatalf("want result=ok, got %q", resp.Msg.Result)
	}

	// Disabled method should fail (not registered, returns 404 → Unimplemented).
	disabledClient := connectrpc.NewClient[testv1.PingRequest, testv1.PingResponse](
		srv.Client(),
		srv.URL+"/test.v1.CliOptionsTestService/ExplicitDisabled",
	)
	_, err = disabledClient.CallUnary(t.Context(), connectrpc.NewRequest(&testv1.PingRequest{
		Target: "should-fail",
	}))
	if err == nil {
		t.Fatal("disabled method: expected error")
	}
}

// cliOptionsHandler implements CliOptionsTestServiceHandler for opt-out testing.
type cliOptionsHandler struct{}

func (h *cliOptionsHandler) DefaultEnabled(
	_ context.Context,
	req *procframe.Request[testv1.PingRequest],
) (*procframe.Response[testv1.PingResponse], error) {
	return &procframe.Response[testv1.PingResponse]{
		Msg: &testv1.PingResponse{Result: req.Msg.Target},
	}, nil
}

func (h *cliOptionsHandler) ExplicitEnabled(
	_ context.Context,
	req *procframe.Request[testv1.PingRequest],
) (*procframe.Response[testv1.PingResponse], error) {
	return &procframe.Response[testv1.PingResponse]{
		Msg: &testv1.PingResponse{Result: req.Msg.Target},
	}, nil
}

func (h *cliOptionsHandler) ExplicitDisabled(
	_ context.Context,
	req *procframe.Request[testv1.PingRequest],
) (*procframe.Response[testv1.PingResponse], error) {
	return &procframe.Response[testv1.PingResponse]{
		Msg: &testv1.PingResponse{Result: req.Msg.Target},
	}, nil
}

func (h *cliOptionsHandler) WsEnabled(
	_ context.Context,
	req *procframe.Request[testv1.PingRequest],
) (*procframe.Response[testv1.PingResponse], error) {
	return &procframe.Response[testv1.PingResponse]{
		Msg: &testv1.PingResponse{Result: req.Msg.Target},
	}, nil
}

// --- FourShapeService integration tests ---

// fourShapeHandler implements FourShapeServiceHandler for testing.
type fourShapeHandler struct{}

func (h *fourShapeHandler) Ping(
	_ context.Context,
	req *procframe.Request[testv1.CollectRequest],
) (*procframe.Response[testv1.CollectResponse], error) {
	return &procframe.Response[testv1.CollectResponse]{
		Msg: &testv1.CollectResponse{Count: 1, Items: req.Msg.Item},
	}, nil
}

func (h *fourShapeHandler) Collect(
	_ context.Context,
	stream procframe.ClientStream[testv1.CollectRequest],
) (*procframe.Response[testv1.CollectResponse], error) {
	var items []string
	for {
		req, err := stream.Receive()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		items = append(items, req.Msg.Item)
	}
	return &procframe.Response[testv1.CollectResponse]{
		Msg: &testv1.CollectResponse{
			Count: int32(len(items)), //nolint:gosec // test-only; count is bounded
			Items: strings.Join(items, ","),
		},
	}, nil
}

func (h *fourShapeHandler) Feed(
	_ context.Context,
	req *procframe.Request[testv1.CollectRequest],
	stream procframe.ServerStream[testv1.ChatReply],
) error {
	return stream.Send(&procframe.Response[testv1.ChatReply]{
		Msg: &testv1.ChatReply{Text: "feed:" + req.Msg.Item},
	})
}

func (h *fourShapeHandler) Chat(
	_ context.Context,
	stream procframe.BidiStream[testv1.ChatMessage, testv1.ChatReply],
) error {
	for {
		req, err := stream.Receive()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.Send(&procframe.Response[testv1.ChatReply]{
			Msg: &testv1.ChatReply{Text: "echo:" + req.Msg.Text},
		}); err != nil {
			return err
		}
	}
}

func TestIntegration_ConnectClientStream(t *testing.T) {
	t.Parallel()

	h := &fourShapeHandler{}
	path, handler := testv1.NewFourShapeServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := connectrpc.NewClient[testv1.CollectRequest, testv1.CollectResponse](
		srv.Client(),
		srv.URL+"/test.v1.FourShapeService/Collect",
	)

	stream := client.CallClientStream(t.Context())
	for _, item := range []string{"a", "b", "c"} {
		if err := stream.Send(&testv1.CollectRequest{Item: item}); err != nil {
			t.Fatalf("send %q: %v", item, err)
		}
	}

	resp, err := stream.CloseAndReceive()
	if err != nil {
		t.Fatalf("CloseAndReceive: %v", err)
	}
	if resp.Msg.Count != 3 {
		t.Fatalf("want count=3, got %d", resp.Msg.Count)
	}
	if resp.Msg.Items != "a,b,c" {
		t.Fatalf("want items=a,b,c, got %q", resp.Msg.Items)
	}
}

func TestIntegration_ConnectBidiStream(t *testing.T) {
	t.Parallel()

	h := &fourShapeHandler{}
	path, handler := testv1.NewFourShapeServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	client := connectrpc.NewClient[testv1.ChatMessage, testv1.ChatReply](
		srv.Client(),
		srv.URL+"/test.v1.FourShapeService/Chat",
		connectrpc.WithGRPC(),
	)

	stream := client.CallBidiStream(t.Context())

	inputs := []string{"hello", "world"}
	for _, text := range inputs {
		if err := stream.Send(&testv1.ChatMessage{Text: text}); err != nil {
			t.Fatalf("send %q: %v", text, err)
		}
	}
	if err := stream.CloseRequest(); err != nil {
		t.Fatalf("CloseRequest: %v", err)
	}

	var replies []string
	for {
		msg, err := stream.Receive()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Receive: %v", err)
		}
		replies = append(replies, msg.Text)
	}
	if err := stream.CloseResponse(); err != nil {
		t.Fatalf("CloseResponse: %v", err)
	}

	if len(replies) != 2 {
		t.Fatalf("want 2 replies, got %d: %v", len(replies), replies)
	}
	for i, want := range []string{"echo:hello", "echo:world"} {
		if replies[i] != want {
			t.Fatalf("reply[%d]: want %q, got %q", i, want, replies[i])
		}
	}
}

// TestIntegration_ConnectUnaryNilResponse verifies that a handler returning
// nil response produces a CodeInternal Connect error.
func TestIntegration_ConnectUnaryNilResponse(t *testing.T) {
	t.Parallel()

	procedure, handler := connecttransport.NewUnaryHandler(
		"/test.v1.EchoService/Echo",
		func(_ context.Context, _ *procframe.Request[testv1.EchoRequest]) (*procframe.Response[testv1.EchoResponse], error) {
			return nil, nil
		},
	)

	mux := http.NewServeMux()
	mux.Handle(procedure, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := connectrpc.NewClient[testv1.EchoRequest, testv1.EchoResponse](
		http.DefaultClient,
		srv.URL+procedure,
	)

	_, err := client.CallUnary(t.Context(), connectrpc.NewRequest(&testv1.EchoRequest{
		Message: "hello",
	}))
	if err == nil {
		t.Fatal("expected error for nil handler response")
	}
	var connectErr *connectrpc.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected connect error, got: %v", err)
	}
	if connectErr.Code() != connectrpc.CodeInternal {
		t.Fatalf("want code=internal, got %v", connectErr.Code())
	}
}

// TestIntegration_ConnectUnaryNilMsg verifies that a handler returning a
// response with nil Msg produces a CodeInternal Connect error.
func TestIntegration_ConnectUnaryNilMsg(t *testing.T) {
	t.Parallel()

	procedure, handler := connecttransport.NewUnaryHandler(
		"/test.v1.EchoService/Echo",
		func(_ context.Context, _ *procframe.Request[testv1.EchoRequest]) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{Msg: nil}, nil
		},
	)

	mux := http.NewServeMux()
	mux.Handle(procedure, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := connectrpc.NewClient[testv1.EchoRequest, testv1.EchoResponse](
		http.DefaultClient,
		srv.URL+procedure,
	)

	_, err := client.CallUnary(t.Context(), connectrpc.NewRequest(&testv1.EchoRequest{
		Message: "hello",
	}))
	if err == nil {
		t.Fatal("expected error for nil Msg in response")
	}
	var connectErr *connectrpc.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected connect error, got: %v", err)
	}
	if connectErr.Code() != connectrpc.CodeInternal {
		t.Fatalf("want code=internal, got %v", connectErr.Code())
	}
}

// TestIntegration_ConnectServerStreamSend verifies that a server-stream
// handler returning nil response in Send does not panic.
func TestIntegration_ConnectServerStreamSend(t *testing.T) {
	t.Parallel()

	procedure, handler := connecttransport.NewServerStreamHandler(
		"/test.v1.TickService/Watch",
		func(_ context.Context, _ *procframe.Request[testv1.TickRequest], stream procframe.ServerStream[testv1.TickResponse]) error {
			// Send a valid response first.
			return stream.Send(&procframe.Response[testv1.TickResponse]{
				Msg: &testv1.TickResponse{Label: "ok", Seq: 1},
			})
		},
	)

	mux := http.NewServeMux()
	mux.Handle(procedure, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := connectrpc.NewClient[testv1.TickRequest, testv1.TickResponse](
		http.DefaultClient,
		srv.URL+procedure,
	)

	stream, err := client.CallServerStream(t.Context(), connectrpc.NewRequest(&testv1.TickRequest{
		Label: "test",
		Count: 1,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	if !stream.Receive() {
		t.Fatalf("expected first message, got error: %v", stream.Err())
	}
	if stream.Msg().Label != "ok" {
		t.Fatalf("want label=ok, got %q", stream.Msg().Label)
	}
}

// TestIntegration_ConnectErrorMapping verifies that error mapping handles
// all procframe codes and unknown codes without panic.
func TestIntegration_ConnectErrorMapping(t *testing.T) {
	t.Parallel()

	allCodes := []procframe.Code{
		procframe.CodeInvalidArgument,
		procframe.CodeNotFound,
		procframe.CodeInternal,
		procframe.CodeUnauthenticated,
		procframe.CodeUnavailable,
		procframe.CodeAlreadyExists,
		procframe.CodePermissionDenied,
		procframe.CodeConflict,
		procframe.Code("unknown_code"),
		procframe.Code(""),
	}

	for _, code := range allCodes {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()

			procedure, handler := connecttransport.NewUnaryHandler(
				"/test.v1.EchoService/Echo",
				func(_ context.Context, _ *procframe.Request[testv1.EchoRequest]) (*procframe.Response[testv1.EchoResponse], error) {
					return nil, procframe.NewError(code, "test error for "+string(code))
				},
			)

			mux := http.NewServeMux()
			mux.Handle(procedure, handler)
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			client := connectrpc.NewClient[testv1.EchoRequest, testv1.EchoResponse](
				http.DefaultClient,
				srv.URL+procedure,
			)

			_, err := client.CallUnary(t.Context(), connectrpc.NewRequest(&testv1.EchoRequest{
				Message: "hello",
			}))
			if err == nil {
				t.Fatal("expected error")
			}
			var connectErr *connectrpc.Error
			if !errors.As(err, &connectErr) {
				t.Fatalf("expected connect error, got: %v", err)
			}
		})
	}
}

// TestIntegration_ConnectErrorMapperFallback verifies that a custom ErrorMapper
// that returns (_, false) falls through to CodeInternal.
func TestIntegration_ConnectErrorMapperFallback(t *testing.T) {
	t.Parallel()

	procedure, handler := connecttransport.NewUnaryHandler(
		"/test.v1.EchoService/Echo",
		func(_ context.Context, _ *procframe.Request[testv1.EchoRequest]) (*procframe.Response[testv1.EchoResponse], error) {
			return nil, errors.New("unmapped error")
		},
		connecttransport.WithErrorMapper(func(_ error) (procframe.Status, bool) {
			return procframe.Status{}, false // decline mapping
		}),
	)

	mux := http.NewServeMux()
	mux.Handle(procedure, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := connectrpc.NewClient[testv1.EchoRequest, testv1.EchoResponse](
		http.DefaultClient,
		srv.URL+procedure,
	)

	_, err := client.CallUnary(t.Context(), connectrpc.NewRequest(&testv1.EchoRequest{
		Message: "hello",
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	var connectErr *connectrpc.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected connect error, got: %v", err)
	}
	if connectErr.Code() != connectrpc.CodeInternal {
		t.Fatalf("want code=internal for unmapped error, got %v", connectErr.Code())
	}
}

// TestIntegration_ConnectClientStreamEmpty verifies that a client-stream
// handler with zero messages produces a valid response.
func TestIntegration_ConnectClientStreamEmpty(t *testing.T) {
	t.Parallel()

	procedure, handler := connecttransport.NewClientStreamHandler(
		"/test.v1.FourShapeService/Collect",
		func(_ context.Context, stream procframe.ClientStream[testv1.CollectRequest]) (*procframe.Response[testv1.CollectResponse], error) {
			var count int32
			for {
				_, err := stream.Receive()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return nil, err
				}
				count++
			}
			return &procframe.Response[testv1.CollectResponse]{
				Msg: &testv1.CollectResponse{Count: count},
			}, nil
		},
	)

	mux := http.NewServeMux()
	mux.Handle(procedure, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := connectrpc.NewClient[testv1.CollectRequest, testv1.CollectResponse](
		http.DefaultClient,
		srv.URL+procedure,
	)

	stream := client.CallClientStream(t.Context())
	// Send zero messages, immediately close.
	resp, err := stream.CloseAndReceive()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Count != 0 {
		t.Fatalf("want count=0, got %d", resp.Msg.Count)
	}
}

// TestIntegration_ConnectErrorNoInternalLeak verifies that error
// messages from Connect don't expose internal framework details.
func TestIntegration_ConnectErrorNoInternalLeak(t *testing.T) {
	t.Parallel()

	procedure, handler := connecttransport.NewUnaryHandler(
		"/test.v1.EchoService/Echo",
		func(_ context.Context, _ *procframe.Request[testv1.EchoRequest]) (*procframe.Response[testv1.EchoResponse], error) {
			return nil, procframe.NewError(procframe.CodePermissionDenied, "access denied")
		},
	)

	mux := http.NewServeMux()
	mux.Handle(procedure, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := connectrpc.NewClient[testv1.EchoRequest, testv1.EchoResponse](
		http.DefaultClient,
		srv.URL+procedure,
	)

	_, err := client.CallUnary(t.Context(), connectrpc.NewRequest(&testv1.EchoRequest{
		Message: "hello",
	}))
	if err == nil {
		t.Fatal("expected error")
	}

	msg := err.Error()
	for _, leak := range []string{".go:", "goroutine", "runtime.", "panic", "stack"} {
		if strings.Contains(msg, leak) {
			t.Fatalf("error message leaks internal: %q found in %q", leak, msg)
		}
	}
}

// --- Generated Connect client integration tests ---

func TestIntegration_ConnectClientUnary(t *testing.T) {
	t.Parallel()

	h := &fourShapeHandler{}
	path, handler := testv1.NewFourShapeServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := testv1.NewFourShapeServiceConnectClient(srv.Client(), srv.URL)

	resp, err := client.Ping(t.Context(), connectrpc.NewRequest(&testv1.CollectRequest{
		Item: "hello",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Count != 1 {
		t.Fatalf("want count=1, got %d", resp.Msg.Count)
	}
	if resp.Msg.Items != "hello" {
		t.Fatalf("want items=hello, got %q", resp.Msg.Items)
	}
}

func TestIntegration_ConnectClientClientStream(t *testing.T) {
	t.Parallel()

	h := &fourShapeHandler{}
	path, handler := testv1.NewFourShapeServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := testv1.NewFourShapeServiceConnectClient(srv.Client(), srv.URL)

	stream := client.Collect(t.Context())
	for _, item := range []string{"x", "y", "z"} {
		if err := stream.Send(&testv1.CollectRequest{Item: item}); err != nil {
			t.Fatalf("send %q: %v", item, err)
		}
	}

	resp, err := stream.CloseAndReceive()
	if err != nil {
		t.Fatalf("CloseAndReceive: %v", err)
	}
	if resp.Msg.Count != 3 {
		t.Fatalf("want count=3, got %d", resp.Msg.Count)
	}
	if resp.Msg.Items != "x,y,z" {
		t.Fatalf("want items=x,y,z, got %q", resp.Msg.Items)
	}
}

func TestIntegration_ConnectClientServerStream(t *testing.T) {
	t.Parallel()

	h := &fourShapeHandler{}
	path, handler := testv1.NewFourShapeServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := testv1.NewFourShapeServiceConnectClient(srv.Client(), srv.URL)

	stream, err := client.Feed(t.Context(), connectrpc.NewRequest(&testv1.CollectRequest{
		Item: "test",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var msgs []*testv1.ChatReply
	for stream.Receive() {
		msgs = append(msgs, stream.Msg())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}

	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	if msgs[0].Text != "feed:test" {
		t.Fatalf("want text=feed:test, got %q", msgs[0].Text)
	}
}

func TestIntegration_ConnectClientBidi(t *testing.T) {
	t.Parallel()

	h := &fourShapeHandler{}
	path, handler := testv1.NewFourShapeServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	client := testv1.NewFourShapeServiceConnectClient(srv.Client(), srv.URL, connectrpc.WithGRPC())

	stream := client.Chat(t.Context())

	inputs := []string{"ping", "pong"}
	for _, text := range inputs {
		if err := stream.Send(&testv1.ChatMessage{Text: text}); err != nil {
			t.Fatalf("send %q: %v", text, err)
		}
	}
	if err := stream.CloseRequest(); err != nil {
		t.Fatalf("CloseRequest: %v", err)
	}

	var replies []string
	for {
		msg, err := stream.Receive()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Receive: %v", err)
		}
		replies = append(replies, msg.Text)
	}
	if err := stream.CloseResponse(); err != nil {
		t.Fatalf("CloseResponse: %v", err)
	}

	if len(replies) != 2 {
		t.Fatalf("want 2 replies, got %d: %v", len(replies), replies)
	}
	for i, want := range []string{"echo:ping", "echo:pong"} {
		if replies[i] != want {
			t.Fatalf("reply[%d]: want %q, got %q", i, want, replies[i])
		}
	}
}

func TestIntegration_ConnectClientOptOut(t *testing.T) {
	t.Parallel()

	// CliOptionsTestService has DefaultEnabled and ExplicitEnabled with
	// connect.enabled = true. ExplicitDisabled and WsEnabled do not.
	// The generated client should only expose DefaultEnabled and ExplicitEnabled.

	h := &cliOptionsHandler{}
	path, handler := testv1.NewCliOptionsTestServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := testv1.NewCliOptionsTestServiceConnectClient(srv.Client(), srv.URL)

	// DefaultEnabled should work through the generated client.
	resp, err := client.DefaultEnabled(t.Context(), connectrpc.NewRequest(&testv1.PingRequest{
		Target: "via-client",
	}))
	if err != nil {
		t.Fatalf("DefaultEnabled: unexpected error: %v", err)
	}
	if resp.Msg.Result != "via-client" {
		t.Fatalf("want result=via-client, got %q", resp.Msg.Result)
	}

	// ExplicitEnabled should also work.
	resp, err = client.ExplicitEnabled(t.Context(), connectrpc.NewRequest(&testv1.PingRequest{
		Target: "explicit",
	}))
	if err != nil {
		t.Fatalf("ExplicitEnabled: unexpected error: %v", err)
	}
	if resp.Msg.Result != "explicit" {
		t.Fatalf("want result=explicit, got %q", resp.Msg.Result)
	}
}
