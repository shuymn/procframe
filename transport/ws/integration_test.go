package ws_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/shuymn/procframe"
	testv1 "github.com/shuymn/procframe/internal/gen/test/v1"
	ws "github.com/shuymn/procframe/transport/ws"
)

// ============================================================
// Test handler implementations
// ============================================================

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

// ============================================================
// Test helpers
// ============================================================

type inboundFrame struct {
	ID        string          `json:"id"`
	Procedure string          `json:"procedure"`
	Payload   json.RawMessage `json:"payload"`
}

type outboundFrame struct {
	ID      string          `json:"id"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *errorDetail    `json:"error,omitempty"`
	EOS     bool            `json:"eos"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func startWSServer(t *testing.T, s *ws.Server) (*websocket.Conn, context.Context) {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle("/ws", s)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ctx := t.Context()
	wsURL := "ws" + srv.URL[len("http"):] + "/ws"
	//nolint:bodyclose // coder/websocket manages resp.Body internally
	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() {
		c.CloseNow() //nolint:errcheck // best-effort cleanup; error not actionable in cleanup
	})

	return c, ctx
}

func sendFrame(
	t *testing.T,
	ctx context.Context, //nolint:revive // testing.T conventionally precedes context in test helpers
	conn *websocket.Conn,
	frame inboundFrame,
) {
	t.Helper()
	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	err = conn.Write(ctx, websocket.MessageText, data)
	if err != nil {
		t.Fatalf("write frame: %v", err)
	}
}

func readFrame(
	t *testing.T,
	ctx context.Context, //nolint:revive // testing.T conventionally precedes context in test helpers
	conn *websocket.Conn,
) outboundFrame {
	t.Helper()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	var out outboundFrame
	if err = json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal frame: %v", err)
	}
	return out
}

// ============================================================
// Integration tests
// ============================================================

func TestIntegration_WSUnarySuccess(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewEchoServiceWSHandler(s, &echoHandler{})

	conn, ctx := startWSServer(t, s)

	payload, err := protojson.Marshal(&testv1.EchoRequest{
		Message:   "hello",
		Count:     3,
		Uppercase: true,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	sendFrame(t, ctx, conn, inboundFrame{
		ID:        "req-1",
		Procedure: "/test.v1.EchoService/Echo",
		Payload:   json.RawMessage(payload),
	})

	out := readFrame(t, ctx, conn)
	if out.ID != "req-1" {
		t.Fatalf("want id=req-1, got %q", out.ID)
	}
	if !out.EOS {
		t.Fatal("want eos=true for unary response")
	}
	if out.Error != nil {
		t.Fatalf("unexpected error: %+v", out.Error)
	}

	var resp testv1.EchoResponse
	if err = protojson.Unmarshal(out.Payload, &resp); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if resp.Message != "HELLO" {
		t.Fatalf("want HELLO, got %q", resp.Message)
	}
	if resp.Count != 3 {
		t.Fatalf("want count=3, got %d", resp.Count)
	}
}

func TestIntegration_WSUnaryError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		procCode  procframe.Code
		retryable bool
	}{
		{procframe.CodeInvalidArgument, false},
		{procframe.CodeNotFound, false},
		{procframe.CodeInternal, false},
		{procframe.CodeUnauthenticated, false},
		{procframe.CodeUnavailable, false},
		{procframe.CodeAlreadyExists, false},
		{procframe.CodePermissionDenied, false},
		{procframe.CodeConflict, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.procCode), func(t *testing.T) {
			t.Parallel()

			s := ws.NewServer()
			ws.HandleUnary(s, "/test.v1.EchoService/Echo",
				func(
					_ context.Context,
					_ *procframe.Request[testv1.EchoRequest],
				) (*procframe.Response[testv1.EchoResponse], error) {
					return nil, procframe.NewError(tt.procCode, "test error")
				},
			)

			conn, ctx := startWSServer(t, s)

			sendFrame(t, ctx, conn, inboundFrame{
				ID:        "err-1",
				Procedure: "/test.v1.EchoService/Echo",
				Payload:   json.RawMessage(`{"message":"test"}`),
			})

			out := readFrame(t, ctx, conn)
			if out.ID != "err-1" {
				t.Fatalf("want id=err-1, got %q", out.ID)
			}
			if !out.EOS {
				t.Fatal("want eos=true")
			}
			if out.Error == nil {
				t.Fatal("expected error frame")
			}
			if out.Error.Code != string(tt.procCode) {
				t.Fatalf("want code=%s, got %s", tt.procCode, out.Error.Code)
			}
			if out.Error.Message != "test error" {
				t.Fatalf("want message='test error', got %q", out.Error.Message)
			}
		})
	}
}

func TestIntegration_WSServerStreaming(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewTickServiceWSHandler(s, &tickHandler{})

	conn, ctx := startWSServer(t, s)

	payload, err := protojson.Marshal(&testv1.TickRequest{Label: "ping", Count: 3})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	sendFrame(t, ctx, conn, inboundFrame{
		ID:        "s-1",
		Procedure: "/test.v1.TickService/Watch",
		Payload:   json.RawMessage(payload),
	})

	// Read 3 data frames (eos=false).
	for i := range 3 {
		out := readFrame(t, ctx, conn)
		if out.ID != "s-1" {
			t.Fatalf("frame %d: want id=s-1, got %q", i, out.ID)
		}
		if out.EOS {
			t.Fatalf("frame %d: want eos=false", i)
		}
		if out.Error != nil {
			t.Fatalf("frame %d: unexpected error: %+v", i, out.Error)
		}
		var tick testv1.TickResponse
		if uErr := protojson.Unmarshal(out.Payload, &tick); uErr != nil {
			t.Fatalf("frame %d: unmarshal: %v", i, uErr)
		}
		if tick.Label != "ping" {
			t.Fatalf("frame %d: want label=ping, got %q", i, tick.Label)
		}
		if tick.Seq != int32(i+1) {
			t.Fatalf("frame %d: want seq=%d, got %d", i, i+1, tick.Seq)
		}
	}

	// Read final EOS frame.
	eos := readFrame(t, ctx, conn)
	if eos.ID != "s-1" {
		t.Fatalf("eos: want id=s-1, got %q", eos.ID)
	}
	if !eos.EOS {
		t.Fatal("eos: want eos=true")
	}
}

func TestIntegration_WSOptOut(t *testing.T) {
	t.Parallel()

	// CliOptionsTestService has 4 methods. Only WsEnabled has ws.enabled = true.
	// The generated handler should only route that one procedure.

	s := ws.NewServer()
	testv1.NewCliOptionsTestServiceWSHandler(s, &cliOptionsHandler{})

	conn, ctx := startWSServer(t, s)

	// WsEnabled method should succeed.
	payload, err := protojson.Marshal(&testv1.PingRequest{Target: "ok"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	sendFrame(t, ctx, conn, inboundFrame{
		ID:        "ws-1",
		Procedure: "/test.v1.CliOptionsTestService/WsEnabled",
		Payload:   json.RawMessage(payload),
	})
	out := readFrame(t, ctx, conn)
	if out.Error != nil {
		t.Fatalf("WsEnabled: unexpected error: %+v", out.Error)
	}
	var resp testv1.PingResponse
	if err = protojson.Unmarshal(out.Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Result != "ok" {
		t.Fatalf("want result=ok, got %q", resp.Result)
	}

	// DefaultEnabled (no ws option) should fail with not_found.
	sendFrame(t, ctx, conn, inboundFrame{
		ID:        "ws-2",
		Procedure: "/test.v1.CliOptionsTestService/DefaultEnabled",
		Payload:   json.RawMessage(payload),
	})
	out2 := readFrame(t, ctx, conn)
	if out2.Error == nil {
		t.Fatal("DefaultEnabled: expected error frame")
	}
	if out2.Error.Code != string(procframe.CodeNotFound) {
		t.Fatalf("want code=%s, got %s", procframe.CodeNotFound, out2.Error.Code)
	}
}

func TestIntegration_WSMultiplexed(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewEchoServiceWSHandler(s, &echoHandler{})

	conn, ctx := startWSServer(t, s)

	// Send 3 requests with different messages.
	cases := []struct {
		id  string
		msg string
	}{
		{"m1", "alpha"},
		{"m2", "bravo"},
		{"m3", "charlie"},
	}
	for _, tc := range cases {
		payload, err := protojson.Marshal(&testv1.EchoRequest{
			Message:   tc.msg,
			Uppercase: true,
		})
		if err != nil {
			t.Fatalf("marshal %s: %v", tc.id, err)
		}
		sendFrame(t, ctx, conn, inboundFrame{
			ID:        tc.id,
			Procedure: "/test.v1.EchoService/Echo",
			Payload:   json.RawMessage(payload),
		})
	}

	// Read all 3 responses (order may vary).
	results := make(map[string]string, 3)
	for range 3 {
		out := readFrame(t, ctx, conn)
		if out.Error != nil {
			t.Fatalf("unexpected error for id=%s: %+v", out.ID, out.Error)
		}
		var resp testv1.EchoResponse
		if uErr := protojson.Unmarshal(out.Payload, &resp); uErr != nil {
			t.Fatalf("unmarshal %s: %v", out.ID, uErr)
		}
		results[out.ID] = resp.Message
	}

	expected := map[string]string{
		"m1": "ALPHA",
		"m2": "BRAVO",
		"m3": "CHARLIE",
	}
	for id, want := range expected {
		if results[id] != want {
			t.Fatalf("id=%s: want %q, got %q", id, want, results[id])
		}
	}
}

func TestIntegration_WSMaxInflight(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 2) // signals that blocking handlers have started
	unblock := make(chan struct{})    // closed to release blocking handlers

	s := ws.NewServer(ws.WithMaxInflight(2))
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			started <- struct{}{}
			<-unblock
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: "done"},
			}, nil
		},
	)

	conn, ctx := startWSServer(t, s)

	// Send 2 requests that will block.
	sendFrame(t, ctx, conn, inboundFrame{
		ID: "a", Procedure: "/test.v1.EchoService/Echo", Payload: json.RawMessage(`{"message":"a"}`),
	})
	sendFrame(t, ctx, conn, inboundFrame{
		ID: "b", Procedure: "/test.v1.EchoService/Echo", Payload: json.RawMessage(`{"message":"b"}`),
	})

	// Wait for both handlers to start (semaphore is full).
	<-started
	<-started

	// Send a 3rd request that exceeds max inflight.
	sendFrame(t, ctx, conn, inboundFrame{
		ID: "c", Procedure: "/test.v1.EchoService/Echo", Payload: json.RawMessage(`{"message":"c"}`),
	})

	// Read the rejection for "c".
	out := readFrame(t, ctx, conn)
	if out.ID != "c" {
		t.Fatalf("want id=c, got %q", out.ID)
	}
	if out.Error == nil {
		t.Fatal("expected error frame for exceeded inflight")
	}
	if out.Error.Code != string(procframe.CodeUnavailable) {
		t.Fatalf("want code=%s, got %s", procframe.CodeUnavailable, out.Error.Code)
	}
	if !out.Error.Retryable {
		t.Fatal("want retryable=true")
	}

	// Unblock the first 2 handlers.
	close(unblock)

	// Read their successful responses.
	completed := make(map[string]bool, 2)
	for range 2 {
		out = readFrame(t, ctx, conn)
		if out.Error != nil {
			t.Fatalf("unexpected error for id=%s: %+v", out.ID, out.Error)
		}
		completed[out.ID] = true
	}
	if !completed["a"] || !completed["b"] {
		t.Fatalf("missing responses: got %v", completed)
	}
}

func TestIntegration_WSDisconnect(t *testing.T) {
	t.Parallel()

	handlerDone := make(chan struct{})

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			ctx context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			// Block until context is cancelled (client disconnect).
			<-ctx.Done()
			close(handlerDone)
			return nil, ctx.Err()
		},
	)

	mux := http.NewServeMux()
	mux.Handle("/ws", s)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := t.Context()
	wsURL := "ws" + srv.URL[len("http"):] + "/ws"
	//nolint:bodyclose // coder/websocket manages resp.Body internally
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	sendFrame(t, ctx, conn, inboundFrame{
		ID:        "d-1",
		Procedure: "/test.v1.EchoService/Echo",
		Payload:   json.RawMessage(`{"message":"block"}`),
	})

	// Give the handler time to start blocking.
	time.Sleep(50 * time.Millisecond)

	// Close the client connection.
	conn.Close(websocket.StatusNormalClosure, "bye")

	select {
	case <-handlerDone:
		// Success: handler detected disconnect.
	case <-time.After(time.Second):
		t.Fatal("handler did not finish within 1 second after client disconnect")
	}
}

func TestIntegration_WSErrorMapper(t *testing.T) {
	t.Parallel()

	errCustom := errors.New("custom domain error")

	s := ws.NewServer(ws.WithErrorMapper(func(err error) (procframe.Status, bool) {
		if errors.Is(err, errCustom) {
			return procframe.Status{
				Code:    procframe.CodePermissionDenied,
				Message: "mapped: " + err.Error(),
			}, true
		}
		return procframe.Status{}, false
	}))
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return nil, errCustom
		},
	)

	conn, ctx := startWSServer(t, s)

	sendFrame(t, ctx, conn, inboundFrame{
		ID:        "em-1",
		Procedure: "/test.v1.EchoService/Echo",
		Payload:   json.RawMessage(`{"message":"test"}`),
	})

	out := readFrame(t, ctx, conn)
	if out.Error == nil {
		t.Fatal("expected error frame")
	}
	if out.Error.Code != string(procframe.CodePermissionDenied) {
		t.Fatalf("want code=%s, got %s", procframe.CodePermissionDenied, out.Error.Code)
	}
	if out.Error.Message != "mapped: custom domain error" {
		t.Fatalf("want mapped message, got %q", out.Error.Message)
	}
}

func TestIntegration_WSUnknownProcedure(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewEchoServiceWSHandler(s, &echoHandler{})

	conn, ctx := startWSServer(t, s)

	sendFrame(t, ctx, conn, inboundFrame{
		ID:        "unk-1",
		Procedure: "/test.v1.EchoService/NonExistent",
		Payload:   json.RawMessage(`{}`),
	})

	out := readFrame(t, ctx, conn)
	if out.Error == nil {
		t.Fatal("expected error frame")
	}
	if out.Error.Code != string(procframe.CodeNotFound) {
		t.Fatalf("want code=%s, got %s", procframe.CodeNotFound, out.Error.Code)
	}
}
