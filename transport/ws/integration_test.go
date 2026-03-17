package ws_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

// fourShapeHandler implements FourShapeServiceHandler for integration tests.
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
			Count: int32(len(items)), //nolint:gosec // G115: len(items) is bounded by test data
			Items: strings.Join(items, ","),
		},
	}, nil
}

func (h *fourShapeHandler) Feed(
	_ context.Context,
	req *procframe.Request[testv1.CollectRequest],
	stream procframe.ServerStream[testv1.ChatReply],
) error {
	parts := strings.Split(req.Msg.Item, ",")
	for _, p := range parts {
		if err := stream.Send(&procframe.Response[testv1.ChatReply]{
			Msg: &testv1.ChatReply{Text: p},
		}); err != nil {
			return err
		}
	}
	return nil
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

// ============================================================
// Test helpers — v2 session protocol
// ============================================================

type inboundFrame struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Procedure string          `json:"procedure,omitempty"`
	Shape     string          `json:"shape,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type outboundFrame struct {
	Type    string          `json:"type"`
	ID      string          `json:"id"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *errorDetail    `json:"error,omitempty"`
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

func sendOpen(
	t *testing.T,
	ctx context.Context, //nolint:revive // testing.T conventionally precedes context in test helpers
	conn *websocket.Conn,
	id, procedure, shape string,
) {
	t.Helper()
	sendFrame(t, ctx, conn, inboundFrame{
		Type: "open", ID: id, Procedure: procedure, Shape: shape,
	})
}

func sendMessage(
	t *testing.T,
	ctx context.Context, //nolint:revive // testing.T conventionally precedes context in test helpers
	conn *websocket.Conn,
	id string,
	payload json.RawMessage,
) {
	t.Helper()
	sendFrame(t, ctx, conn, inboundFrame{
		Type: "message", ID: id, Payload: payload,
	})
}

func sendClose(
	t *testing.T,
	ctx context.Context, //nolint:revive // testing.T conventionally precedes context in test helpers
	conn *websocket.Conn,
	id string,
) {
	t.Helper()
	sendFrame(t, ctx, conn, inboundFrame{Type: "close", ID: id})
}

func sendCancel(
	t *testing.T,
	ctx context.Context, //nolint:revive // testing.T conventionally precedes context in test helpers
	conn *websocket.Conn,
	id string,
) {
	t.Helper()
	sendFrame(t, ctx, conn, inboundFrame{Type: "cancel", ID: id})
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
// Integration tests — Unary
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

	sendOpen(t, ctx, conn, "req-1", "/test.v1.EchoService/Echo", "unary")
	sendMessage(t, ctx, conn, "req-1", json.RawMessage(payload))
	sendClose(t, ctx, conn, "req-1")

	out := readFrame(t, ctx, conn)
	if out.ID != "req-1" {
		t.Fatalf("want id=req-1, got %q", out.ID)
	}
	if out.Type != "message" {
		t.Fatalf("want type=message, got %q", out.Type)
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

	// Read close frame.
	close1 := readFrame(t, ctx, conn)
	if close1.Type != "close" {
		t.Fatalf("want type=close, got %q", close1.Type)
	}
	if close1.ID != "req-1" {
		t.Fatalf("want id=req-1, got %q", close1.ID)
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

			sendOpen(t, ctx, conn, "err-1", "/test.v1.EchoService/Echo", "unary")
			sendMessage(t, ctx, conn, "err-1", json.RawMessage(`{"message":"test"}`))
			sendClose(t, ctx, conn, "err-1")

			out := readFrame(t, ctx, conn)
			if out.ID != "err-1" {
				t.Fatalf("want id=err-1, got %q", out.ID)
			}
			if out.Type != "error" {
				t.Fatalf("want type=error, got %q", out.Type)
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

// ============================================================
// Integration tests — Server Streaming
// ============================================================

func TestIntegration_WSServerStreaming(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewTickServiceWSHandler(s, &tickHandler{})

	conn, ctx := startWSServer(t, s)

	payload, err := protojson.Marshal(&testv1.TickRequest{Label: "ping", Count: 3})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	sendOpen(t, ctx, conn, "s-1", "/test.v1.TickService/Watch", "server_stream")
	sendMessage(t, ctx, conn, "s-1", json.RawMessage(payload))
	sendClose(t, ctx, conn, "s-1")

	// Read 3 data frames.
	for i := range 3 {
		out := readFrame(t, ctx, conn)
		if out.ID != "s-1" {
			t.Fatalf("frame %d: want id=s-1, got %q", i, out.ID)
		}
		if out.Type != "message" {
			t.Fatalf("frame %d: want type=message, got %q", i, out.Type)
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

	// Read final close frame.
	eos := readFrame(t, ctx, conn)
	if eos.ID != "s-1" {
		t.Fatalf("close: want id=s-1, got %q", eos.ID)
	}
	if eos.Type != "close" {
		t.Fatalf("close: want type=close, got %q", eos.Type)
	}
}

// ============================================================
// Integration tests — Client Streaming
// ============================================================

func TestIntegration_WSClientStream(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewFourShapeServiceWSHandler(s, &fourShapeHandler{})

	conn, ctx := startWSServer(t, s)

	sendOpen(t, ctx, conn, "cs-1", "/test.v1.FourShapeService/Collect", "client_stream")
	sendMessage(t, ctx, conn, "cs-1", json.RawMessage(`{"item":"alpha"}`))
	sendMessage(t, ctx, conn, "cs-1", json.RawMessage(`{"item":"bravo"}`))
	sendMessage(t, ctx, conn, "cs-1", json.RawMessage(`{"item":"charlie"}`))
	sendClose(t, ctx, conn, "cs-1")

	// Read response message.
	out := readFrame(t, ctx, conn)
	if out.ID != "cs-1" {
		t.Fatalf("want id=cs-1, got %q", out.ID)
	}
	if out.Type != "message" {
		t.Fatalf("want type=message, got %q", out.Type)
	}
	if out.Error != nil {
		t.Fatalf("unexpected error: %+v", out.Error)
	}

	var resp testv1.CollectResponse
	if err := protojson.Unmarshal(out.Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Count != 3 {
		t.Fatalf("want count=3, got %d", resp.Count)
	}
	if resp.Items != "alpha,bravo,charlie" {
		t.Fatalf("want items=alpha,bravo,charlie, got %q", resp.Items)
	}

	// Read close frame.
	close1 := readFrame(t, ctx, conn)
	if close1.Type != "close" {
		t.Fatalf("want type=close, got %q", close1.Type)
	}
}

func TestIntegration_WSClientStreamEmpty(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewFourShapeServiceWSHandler(s, &fourShapeHandler{})

	conn, ctx := startWSServer(t, s)

	// Send open then immediately close (zero messages).
	sendOpen(t, ctx, conn, "cs-0", "/test.v1.FourShapeService/Collect", "client_stream")
	sendClose(t, ctx, conn, "cs-0")

	out := readFrame(t, ctx, conn)
	if out.Type != "message" {
		t.Fatalf("want type=message, got %q", out.Type)
	}

	var resp testv1.CollectResponse
	if err := protojson.Unmarshal(out.Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Count != 0 {
		t.Fatalf("want count=0, got %d", resp.Count)
	}

	close1 := readFrame(t, ctx, conn)
	if close1.Type != "close" {
		t.Fatalf("want type=close, got %q", close1.Type)
	}
}

// ============================================================
// Integration tests — Bidi Streaming
// ============================================================

func TestIntegration_WSBidi(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewFourShapeServiceWSHandler(s, &fourShapeHandler{})

	conn, ctx := startWSServer(t, s)

	sendOpen(t, ctx, conn, "bi-1", "/test.v1.FourShapeService/Chat", "bidi")

	// Send messages and read responses interleaved.
	msgs := []string{"hello", "world"}
	for _, m := range msgs {
		sendMessage(t, ctx, conn, "bi-1", json.RawMessage(`{"text":"`+m+`"}`))

		out := readFrame(t, ctx, conn)
		if out.ID != "bi-1" {
			t.Fatalf("want id=bi-1, got %q", out.ID)
		}
		if out.Type != "message" {
			t.Fatalf("want type=message, got %q", out.Type)
		}
		var reply testv1.ChatReply
		if err := protojson.Unmarshal(out.Payload, &reply); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if reply.Text != "echo:"+m {
			t.Fatalf("want echo:%s, got %q", m, reply.Text)
		}
	}

	sendClose(t, ctx, conn, "bi-1")

	// Read close frame.
	close1 := readFrame(t, ctx, conn)
	if close1.Type != "close" {
		t.Fatalf("want type=close, got %q", close1.Type)
	}
}

// ============================================================
// Integration tests — Server Streaming (FourShape Feed)
// ============================================================

func TestIntegration_WSFourShapeFeed(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewFourShapeServiceWSHandler(s, &fourShapeHandler{})

	conn, ctx := startWSServer(t, s)

	sendOpen(t, ctx, conn, "f-1", "/test.v1.FourShapeService/Feed", "server_stream")
	sendMessage(t, ctx, conn, "f-1", json.RawMessage(`{"item":"a,b,c"}`))
	sendClose(t, ctx, conn, "f-1")

	// Read 3 reply messages.
	expected := []string{"a", "b", "c"}
	for i, want := range expected {
		out := readFrame(t, ctx, conn)
		if out.Type != "message" {
			t.Fatalf("frame %d: want type=message, got %q", i, out.Type)
		}
		var reply testv1.ChatReply
		if err := protojson.Unmarshal(out.Payload, &reply); err != nil {
			t.Fatalf("frame %d: unmarshal: %v", i, err)
		}
		if reply.Text != want {
			t.Fatalf("frame %d: want %q, got %q", i, want, reply.Text)
		}
	}

	close1 := readFrame(t, ctx, conn)
	if close1.Type != "close" {
		t.Fatalf("want type=close, got %q", close1.Type)
	}
}

// ============================================================
// Integration tests — Protocol violations
// ============================================================

func TestIntegration_WSUnaryExtraMessage(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			req *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: req.Msg.Message},
			}, nil
		},
	)

	conn, ctx := startWSServer(t, s)

	sendOpen(t, ctx, conn, "extra-1", "/test.v1.EchoService/Echo", "unary")
	sendMessage(t, ctx, conn, "extra-1", json.RawMessage(`{"message":"a"}`))
	sendMessage(t, ctx, conn, "extra-1", json.RawMessage(`{"message":"b"}`))
	sendClose(t, ctx, conn, "extra-1")

	out := readFrame(t, ctx, conn)
	if out.Type != "error" {
		t.Fatalf("want type=error, got %q", out.Type)
	}
	if out.Error == nil {
		t.Fatal("expected error")
	}
	if out.Error.Code != string(procframe.CodeInvalidArgument) {
		t.Fatalf("want code=%s, got %s", procframe.CodeInvalidArgument, out.Error.Code)
	}
}

func TestIntegration_WSShapeMismatch(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: "ok"},
			}, nil
		},
	)

	conn, ctx := startWSServer(t, s)

	// Open with wrong shape.
	sendOpen(t, ctx, conn, "sm-1", "/test.v1.EchoService/Echo", "bidi")

	out := readFrame(t, ctx, conn)
	if out.Type != "error" {
		t.Fatalf("want type=error, got %q", out.Type)
	}
	if out.Error.Code != string(procframe.CodeInvalidArgument) {
		t.Fatalf("want code=%s, got %s", procframe.CodeInvalidArgument, out.Error.Code)
	}
}

func TestIntegration_WSUnaryCloseWithoutMessage(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: "ok"},
			}, nil
		},
	)

	conn, ctx := startWSServer(t, s)

	sendOpen(t, ctx, conn, "noreq-1", "/test.v1.EchoService/Echo", "unary")
	sendClose(t, ctx, conn, "noreq-1") // close without any message

	out := readFrame(t, ctx, conn)
	if out.Type != "error" {
		t.Fatalf("want type=error, got %q", out.Type)
	}
	if out.Error.Code != string(procframe.CodeInvalidArgument) {
		t.Fatalf("want code=%s, got %s", procframe.CodeInvalidArgument, out.Error.Code)
	}
}

// ============================================================
// Integration tests — Opt-out
// ============================================================

func TestIntegration_WSOptOut(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewCliOptionsTestServiceWSHandler(s, &cliOptionsHandler{})

	conn, ctx := startWSServer(t, s)

	// WsEnabled method should succeed.
	payload, err := protojson.Marshal(&testv1.PingRequest{Target: "ok"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	sendOpen(t, ctx, conn, "ws-1", "/test.v1.CliOptionsTestService/WsEnabled", "unary")
	sendMessage(t, ctx, conn, "ws-1", json.RawMessage(payload))
	sendClose(t, ctx, conn, "ws-1")

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

	// Read close frame.
	close1 := readFrame(t, ctx, conn)
	if close1.Type != "close" {
		t.Fatalf("want type=close, got %q", close1.Type)
	}

	// DefaultEnabled (no ws option) should fail with not_found.
	sendOpen(t, ctx, conn, "ws-2", "/test.v1.CliOptionsTestService/DefaultEnabled", "unary")

	out2 := readFrame(t, ctx, conn)
	if out2.Error == nil {
		t.Fatal("DefaultEnabled: expected error frame")
	}
	if out2.Error.Code != string(procframe.CodeNotFound) {
		t.Fatalf("want code=%s, got %s", procframe.CodeNotFound, out2.Error.Code)
	}
}

// ============================================================
// Integration tests — Multiplexing
// ============================================================

func TestIntegration_WSMultiplexed(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewEchoServiceWSHandler(s, &echoHandler{})

	conn, ctx := startWSServer(t, s)

	// Open 3 sessions in parallel.
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
		sendOpen(t, ctx, conn, tc.id, "/test.v1.EchoService/Echo", "unary")
		sendMessage(t, ctx, conn, tc.id, json.RawMessage(payload))
		sendClose(t, ctx, conn, tc.id)
	}

	// Read all responses (message + close per session, order may vary).
	results := make(map[string]string, 3)
	for range 6 { // 3 message + 3 close
		out := readFrame(t, ctx, conn)
		if out.Error != nil {
			t.Fatalf("unexpected error for id=%s: %+v", out.ID, out.Error)
		}
		if out.Type == "message" {
			var resp testv1.EchoResponse
			if uErr := protojson.Unmarshal(out.Payload, &resp); uErr != nil {
				t.Fatalf("unmarshal %s: %v", out.ID, uErr)
			}
			results[out.ID] = resp.Message
		}
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

// ============================================================
// Integration tests — Max inflight
// ============================================================

func TestIntegration_WSMaxInflight(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 2)
	unblock := make(chan struct{})

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

	// Open 2 sessions that will block.
	sendOpen(t, ctx, conn, "a", "/test.v1.EchoService/Echo", "unary")
	sendMessage(t, ctx, conn, "a", json.RawMessage(`{"message":"a"}`))
	sendClose(t, ctx, conn, "a")

	sendOpen(t, ctx, conn, "b", "/test.v1.EchoService/Echo", "unary")
	sendMessage(t, ctx, conn, "b", json.RawMessage(`{"message":"b"}`))
	sendClose(t, ctx, conn, "b")

	// Wait for both handlers to start.
	<-started
	<-started

	// 3rd session should be rejected.
	sendOpen(t, ctx, conn, "c", "/test.v1.EchoService/Echo", "unary")

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

	// Read their successful responses (message + close each).
	completed := make(map[string]bool, 2)
	for range 4 { // 2 message + 2 close
		out = readFrame(t, ctx, conn)
		if out.Error != nil {
			t.Fatalf("unexpected error for id=%s: %+v", out.ID, out.Error)
		}
		if out.Type == "message" {
			completed[out.ID] = true
		}
	}
	if !completed["a"] || !completed["b"] {
		t.Fatalf("missing responses: got %v", completed)
	}
}

// ============================================================
// Integration tests — Disconnect
// ============================================================

func TestIntegration_WSDisconnect(t *testing.T) {
	t.Parallel()

	handlerDone := make(chan struct{})

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			ctx context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
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

	sendOpen(t, ctx, conn, "d-1", "/test.v1.EchoService/Echo", "unary")
	sendMessage(t, ctx, conn, "d-1", json.RawMessage(`{"message":"block"}`))
	sendClose(t, ctx, conn, "d-1")

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

// ============================================================
// Integration tests — Cancel
// ============================================================

func TestIntegration_WSCancel(t *testing.T) {
	t.Parallel()

	handlerDone := make(chan struct{})

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			ctx context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			<-ctx.Done()
			close(handlerDone)
			return nil, ctx.Err()
		},
	)

	conn, ctx := startWSServer(t, s)

	sendOpen(t, ctx, conn, "cancel-1", "/test.v1.EchoService/Echo", "unary")
	sendMessage(t, ctx, conn, "cancel-1", json.RawMessage(`{"message":"block"}`))
	sendClose(t, ctx, conn, "cancel-1")

	// Give the handler time to start blocking.
	time.Sleep(50 * time.Millisecond)

	sendCancel(t, ctx, conn, "cancel-1")

	select {
	case <-handlerDone:
		// Success: handler detected cancel.
	case <-time.After(time.Second):
		t.Fatal("handler did not finish within 1 second after cancel")
	}
}

// ============================================================
// Integration tests — Error mapper
// ============================================================

func TestIntegration_WSErrorMapper(t *testing.T) {
	t.Parallel()

	errCustom := errors.New("custom domain error")

	s := ws.NewServer(ws.WithErrorMapper(func(err error) (*procframe.Status, bool) {
		if errors.Is(err, errCustom) {
			return &procframe.Status{
				Code:    procframe.CodePermissionDenied,
				Message: "mapped: " + err.Error(),
			}, true
		}
		return nil, false
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

	sendOpen(t, ctx, conn, "em-1", "/test.v1.EchoService/Echo", "unary")
	sendMessage(t, ctx, conn, "em-1", json.RawMessage(`{"message":"test"}`))
	sendClose(t, ctx, conn, "em-1")

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

// ============================================================
// Integration tests — Unknown procedure
// ============================================================

func TestIntegration_WSUnknownProcedure(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewEchoServiceWSHandler(s, &echoHandler{})

	conn, ctx := startWSServer(t, s)

	sendOpen(t, ctx, conn, "unk-1", "/test.v1.EchoService/NonExistent", "unary")

	out := readFrame(t, ctx, conn)
	if out.Error == nil {
		t.Fatal("expected error frame")
	}
	if out.Error.Code != string(procframe.CodeNotFound) {
		t.Fatalf("want code=%s, got %s", procframe.CodeNotFound, out.Error.Code)
	}
}

// ============================================================
// Integration tests — Multiplexing across shapes
// ============================================================

func TestIntegration_WSMixedShapes(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewFourShapeServiceWSHandler(s, &fourShapeHandler{})

	conn, ctx := startWSServer(t, s)

	// Open unary and client-stream concurrently.
	sendOpen(t, ctx, conn, "u1", "/test.v1.FourShapeService/Ping", "unary")
	sendOpen(t, ctx, conn, "cs1", "/test.v1.FourShapeService/Collect", "client_stream")

	sendMessage(t, ctx, conn, "u1", json.RawMessage(`{"item":"ping-item"}`))
	sendClose(t, ctx, conn, "u1")

	sendMessage(t, ctx, conn, "cs1", json.RawMessage(`{"item":"x"}`))
	sendMessage(t, ctx, conn, "cs1", json.RawMessage(`{"item":"y"}`))
	sendClose(t, ctx, conn, "cs1")

	// Read all responses (4 frames: message+close for each).
	gotMessage := make(map[string]bool)
	gotClose := make(map[string]bool)
	for range 4 {
		out := readFrame(t, ctx, conn)
		if out.Error != nil {
			t.Fatalf("unexpected error for id=%s: %+v", out.ID, out.Error)
		}
		switch out.Type {
		case "message":
			gotMessage[out.ID] = true
		case "close":
			gotClose[out.ID] = true
		}
	}
	if !gotMessage["u1"] || !gotMessage["cs1"] {
		t.Fatalf("missing messages: %v", gotMessage)
	}
	if !gotClose["u1"] || !gotClose["cs1"] {
		t.Fatalf("missing closes: %v", gotClose)
	}
}

// ============================================================
// Integration tests — Adversarial: Empty/null values
// ============================================================

// TestIntegration_WSEmptySessionID verifies that an open frame with an empty
// session ID does not panic and returns an appropriate error or is ignored.
func TestIntegration_WSEmptySessionID(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			req *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: req.Msg.Message},
			}, nil
		},
	)

	conn, ctx := startWSServer(t, s)

	// Open with empty session ID.
	sendOpen(t, ctx, conn, "", "/test.v1.EchoService/Echo", "unary")
	sendMessage(t, ctx, conn, "", json.RawMessage(`{"message":"hello"}`))
	sendClose(t, ctx, conn, "")

	// Should not panic; expect a response (message+close or error).
	out := readFrame(t, ctx, conn)
	if out.Type != "message" && out.Type != "error" {
		t.Fatalf("want type=message or type=error, got %q", out.Type)
	}
}

// TestIntegration_WSEmptyProcedure verifies that an open frame with an empty
// procedure name returns a not_found error.
func TestIntegration_WSEmptyProcedure(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: "ok"},
			}, nil
		},
	)

	conn, ctx := startWSServer(t, s)

	sendOpen(t, ctx, conn, "empty-proc-1", "", "unary")

	out := readFrame(t, ctx, conn)
	if out.Type != "error" {
		t.Fatalf("want type=error, got %q", out.Type)
	}
	if out.Error == nil {
		t.Fatal("expected error frame")
	}
	if out.Error.Code != string(procframe.CodeNotFound) {
		t.Fatalf("want code=%s, got %s", procframe.CodeNotFound, out.Error.Code)
	}
}

// TestIntegration_WSNilPayload verifies that sending a message frame with a
// nil/empty payload to a unary handler returns an appropriate error.
func TestIntegration_WSNilPayload(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			req *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: req.Msg.Message},
			}, nil
		},
	)

	conn, ctx := startWSServer(t, s)

	sendOpen(t, ctx, conn, "nil-1", "/test.v1.EchoService/Echo", "unary")
	// Send nil/empty payload.
	sendMessage(t, ctx, conn, "nil-1", nil)
	sendClose(t, ctx, conn, "nil-1")

	out := readFrame(t, ctx, conn)
	if out.Type != "error" && out.Type != "message" {
		t.Fatalf("want type=error or type=message, got %q", out.Type)
	}
	// If error, should not panic and should have a valid error code.
	if out.Type == "error" && out.Error != nil && out.Error.Code == "" {
		t.Fatal("error frame has empty code")
	}
}

// TestIntegration_WSEmptyShape verifies that an open frame with an empty shape
// returns a shape mismatch error.
func TestIntegration_WSEmptyShape(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: "ok"},
			}, nil
		},
	)

	conn, ctx := startWSServer(t, s)

	sendOpen(t, ctx, conn, "shape-1", "/test.v1.EchoService/Echo", "")

	out := readFrame(t, ctx, conn)
	if out.Type != "error" {
		t.Fatalf("want type=error, got %q", out.Type)
	}
	if out.Error == nil {
		t.Fatal("expected error frame")
	}
	if out.Error.Code != string(procframe.CodeInvalidArgument) {
		t.Fatalf("want code=%s, got %s", procframe.CodeInvalidArgument, out.Error.Code)
	}
}

// TestIntegration_WSMessageNonexistentSession verifies that sending a message
// to a non-existent session returns an error frame.
func TestIntegration_WSMessageNonexistentSession(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: "ok"},
			}, nil
		},
	)

	conn, ctx := startWSServer(t, s)

	// Send message without opening a session.
	sendMessage(t, ctx, conn, "ghost-session", json.RawMessage(`{"message":"hello"}`))

	out := readFrame(t, ctx, conn)
	if out.Type != "error" {
		t.Fatalf("want type=error, got %q", out.Type)
	}
	if out.Error == nil {
		t.Fatal("expected error frame")
	}
	if out.Error.Code != string(procframe.CodeNotFound) {
		t.Fatalf("want code=%s, got %s", procframe.CodeNotFound, out.Error.Code)
	}
}

// TestIntegration_WSDuplicateSessionID verifies that opening two sessions with
// the same ID returns an already_exists error.
func TestIntegration_WSDuplicateSessionID(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			ctx context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			started <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		},
	)

	conn, ctx := startWSServer(t, s)

	// Open first session.
	sendOpen(t, ctx, conn, "dup-1", "/test.v1.EchoService/Echo", "unary")
	sendMessage(t, ctx, conn, "dup-1", json.RawMessage(`{"message":"hello"}`))
	sendClose(t, ctx, conn, "dup-1")

	// Wait for handler to start.
	<-started

	// Try to open a second session with the same ID while the first is still running.
	sendOpen(t, ctx, conn, "dup-1", "/test.v1.EchoService/Echo", "unary")

	out := readFrame(t, ctx, conn)
	if out.Type != "error" {
		t.Fatalf("want type=error, got %q", out.Type)
	}
	if out.Error == nil {
		t.Fatal("expected error frame")
	}
	if out.Error.Code != string(procframe.CodeAlreadyExists) {
		t.Fatalf("want code=%s, got %s", procframe.CodeAlreadyExists, out.Error.Code)
	}
}

// ============================================================
// Integration tests — Adversarial: Injection
// ============================================================

// TestIntegration_WSMalformedJSON verifies that completely malformed JSON
// frames are safely discarded without panic.
func TestIntegration_WSMalformedJSON(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			req *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: req.Msg.Message},
			}, nil
		},
	)

	conn, ctx := startWSServer(t, s)

	// Send malformed JSON.
	malformed := []byte(`{not valid json!!!}`)
	if err := conn.Write(ctx, websocket.MessageText, malformed); err != nil {
		t.Fatalf("write malformed: %v", err)
	}

	// Server should silently discard the malformed frame.
	// Verify by sending a valid request afterwards.
	sendOpen(t, ctx, conn, "valid-1", "/test.v1.EchoService/Echo", "unary")
	sendMessage(t, ctx, conn, "valid-1", json.RawMessage(`{"message":"ok"}`))
	sendClose(t, ctx, conn, "valid-1")

	out := readFrame(t, ctx, conn)
	if out.Type != "message" {
		t.Fatalf("want type=message after malformed frame, got %q (error: %+v)", out.Type, out.Error)
	}
}

// TestIntegration_WSSpecialCharactersInProcedure verifies that procedure names
// with path traversal or injection-like characters are handled safely.
func TestIntegration_WSSpecialCharactersInProcedure(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: "ok"},
			}, nil
		},
	)

	conn, ctx := startWSServer(t, s)

	payloads := []string{
		"/../../../etc/passwd",
		"/test.v1.EchoService/Echo; DROP TABLE users;--",
		"/test.v1.EchoService/Echo\x00",
		"<script>alert(1)</script>",
		strings.Repeat("A", 10000),
	}

	for i, proc := range payloads {
		id := json.RawMessage([]byte(`"inj-` + string(rune('0'+i)) + `"`))
		_ = id
		sendOpen(t, ctx, conn, "inj-"+string(rune('0'+i)), proc, "unary")

		out := readFrame(t, ctx, conn)
		if out.Type != "error" {
			t.Fatalf("payload %d: want type=error for injected procedure, got %q", i, out.Type)
		}
		if out.Error == nil {
			t.Fatalf("payload %d: expected error frame", i)
		}
		// Error code should be not_found (no matching handler).
		if out.Error.Code != string(procframe.CodeNotFound) {
			t.Fatalf("payload %d: want code=%s, got %s", i, procframe.CodeNotFound, out.Error.Code)
		}
	}
}

// TestIntegration_WSMalformedPayload verifies that sending a payload
// that cannot be unmarshaled into a proto message returns invalid_argument.
func TestIntegration_WSMalformedPayload(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			req *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: req.Msg.Message},
			}, nil
		},
	)

	conn, ctx := startWSServer(t, s)

	payloads := []json.RawMessage{
		json.RawMessage(`{"unknownField123": "value"}`),
		json.RawMessage(`{"message": 12345}`),
	}

	for i, payload := range payloads {
		id := "bad-" + string(rune('0'+i))
		sendOpen(t, ctx, conn, id, "/test.v1.EchoService/Echo", "unary")
		sendMessage(t, ctx, conn, id, payload)
		sendClose(t, ctx, conn, id)

		out := readFrame(t, ctx, conn)
		if out.Type != "error" {
			t.Fatalf("payload %d: want type=error, got %q", i, out.Type)
		}
		if out.Error == nil {
			t.Fatalf("payload %d: expected error frame", i)
		}
		if out.Error.Code != string(procframe.CodeInvalidArgument) {
			t.Fatalf("payload %d: want code=%s, got %s", i, procframe.CodeInvalidArgument, out.Error.Code)
		}
	}

	// Send a completely non-JSON payload directly via raw write.
	// This tests the frame-level JSON parsing, not proto unmarshal.
	sendOpen(t, ctx, conn, "bad-raw", "/test.v1.EchoService/Echo", "unary")
	rawFrame := []byte(`{"type":"message","id":"bad-raw","payload":not-valid}`)
	if err := conn.Write(ctx, websocket.MessageText, rawFrame); err != nil {
		t.Fatalf("write raw frame: %v", err)
	}
	sendClose(t, ctx, conn, "bad-raw")

	// The server should discard the malformed frame and the session should
	// receive the close, resulting in a "session closed without request" error.
	out := readFrame(t, ctx, conn)
	if out.Type != "error" {
		t.Fatalf("raw payload: want type=error, got %q", out.Type)
	}
}

// ============================================================
// Integration tests — Adversarial: TOCTOU / Race conditions
// ============================================================

// TestIntegration_WSConcurrentSessionsRace exercises concurrent session
// lifecycle operations to detect data races. This test should be run
// with -race to verify thread safety.
func TestIntegration_WSConcurrentSessionsRace(t *testing.T) {
	t.Parallel()

	s := ws.NewServer(ws.WithMaxInflight(100))
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			req *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: req.Msg.Message},
			}, nil
		},
	)

	mux := http.NewServeMux()
	mux.Handle("/ws", s)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ctx := t.Context()
	wsURL := "ws" + srv.URL[len("http"):] + "/ws"

	const numClients = 5
	const sessionsPerClient = 10

	var wg sync.WaitGroup
	errCh := make(chan error, numClients)

	for c := range numClients {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			//nolint:bodyclose // coder/websocket manages resp.Body internally
			conn, _, err := websocket.Dial(ctx, wsURL, nil)
			if err != nil {
				errCh <- err
				return
			}
			defer conn.CloseNow() //nolint:errcheck // best-effort cleanup

			for s := range sessionsPerClient {
				id := json.Number(
					string(rune('A'+clientID)), //nolint:gosec // G115: bounded by test constants
				) + json.Number(
					string(rune('0'+s)),
				)
				sessionID := string(id)
				sendOpen(t, ctx, conn, sessionID, "/test.v1.EchoService/Echo", "unary")
				sendMessage(t, ctx, conn, sessionID, json.RawMessage(`{"message":"test"}`))
				sendClose(t, ctx, conn, sessionID)
			}

			// Read all responses.
			for range sessionsPerClient * 2 { // message + close per session
				readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				_, _, rErr := conn.Read(readCtx)
				cancel()
				if rErr != nil {
					errCh <- rErr
					return
				}
			}
		}(c)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent session error: %v", err)
	}
}

// TestIntegration_WSBidiRace exercises concurrent Receive/Send on a bidi
// stream to detect data races. Must be run with -race.
func TestIntegration_WSBidiRace(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()

	ws.HandleBidi(s, "/test.v1.FourShapeService/Chat",
		func(
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
		},
	)

	conn, ctx := startWSServer(t, s)

	sendOpen(t, ctx, conn, "bidi-race", "/test.v1.FourShapeService/Chat", "bidi")

	const msgCount = 20
	for i := range msgCount {
		msg := json.RawMessage(`{"text":"msg-` + string(rune('A'+i%26)) + `"}`)
		sendMessage(t, ctx, conn, "bidi-race", msg)
	}

	// Read responses for all sent messages.
	for range msgCount {
		out := readFrame(t, ctx, conn)
		if out.Type != "message" {
			t.Fatalf("want type=message, got %q (error: %+v)", out.Type, out.Error)
		}
	}

	sendClose(t, ctx, conn, "bidi-race")

	closeFrame := readFrame(t, ctx, conn)
	if closeFrame.Type != "close" {
		t.Fatalf("want type=close, got %q", closeFrame.Type)
	}
}

// TestIntegration_WSRapidOpenClose exercises rapid session open and immediate
// close to probe for race conditions in session lifecycle management.
func TestIntegration_WSRapidOpenClose(t *testing.T) {
	t.Parallel()

	s := ws.NewServer(ws.WithMaxInflight(100))
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			req *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: req.Msg.Message},
			}, nil
		},
	)

	conn, ctx := startWSServer(t, s)

	const iterations = 50
	for i := range iterations {
		id := "rapid-" + string(rune('A'+i/26)) + string(rune('a'+i%26))
		sendOpen(t, ctx, conn, id, "/test.v1.EchoService/Echo", "unary")
		sendMessage(t, ctx, conn, id, json.RawMessage(`{"message":"test"}`))
		sendClose(t, ctx, conn, id)
	}

	// Read all responses.
	responded := 0
	readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for responded < iterations*2 {
		_, _, err := conn.Read(readCtx)
		if err != nil {
			break
		}
		responded++
	}

	if responded < iterations {
		t.Fatalf("expected at least %d responses, got %d", iterations, responded)
	}
}

// ============================================================
// WS Client — Helpers
// ============================================================

func startWSClientConn(t *testing.T, s *ws.Server) *ws.Conn {
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

	conn := ws.NewConn(ctx, c)
	return conn
}

// ============================================================
// WS Client — Unary
// ============================================================

func TestIntegration_WSClientUnary(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewFourShapeServiceWSHandler(s, &fourShapeHandler{})

	conn := startWSClientConn(t, s)
	client := testv1.NewFourShapeServiceWSClient(conn)

	resp, err := client.Ping(t.Context(), &testv1.CollectRequest{Item: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("want count=1, got %d", resp.Count)
	}
	if resp.Items != "hello" {
		t.Fatalf("want items=hello, got %q", resp.Items)
	}
}

// ============================================================
// WS Client — Server Stream
// ============================================================

func TestIntegration_WSClientServerStream(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewFourShapeServiceWSHandler(s, &fourShapeHandler{})

	conn := startWSClientConn(t, s)
	client := testv1.NewFourShapeServiceWSClient(conn)

	stream, err := client.Feed(t.Context(), &testv1.CollectRequest{Item: "a,b,c"})
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}

	var msgs []string
	for {
		msg, rErr := stream.Receive()
		if errors.Is(rErr, io.EOF) {
			break
		}
		if rErr != nil {
			t.Fatalf("Receive: %v", rErr)
		}
		msgs = append(msgs, msg.Text)
	}

	if len(msgs) != 3 {
		t.Fatalf("want 3 messages, got %d: %v", len(msgs), msgs)
	}
	expected := []string{"a", "b", "c"}
	for i, want := range expected {
		if msgs[i] != want {
			t.Fatalf("msg[%d]: want %q, got %q", i, want, msgs[i])
		}
	}
}

// ============================================================
// WS Client — Client Stream
// ============================================================

func TestIntegration_WSClientClientStream(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewFourShapeServiceWSHandler(s, &fourShapeHandler{})

	conn := startWSClientConn(t, s)
	client := testv1.NewFourShapeServiceWSClient(conn)

	stream, err := client.Collect(t.Context())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	for _, item := range []string{"x", "y", "z"} {
		if sErr := stream.Send(&testv1.CollectRequest{Item: item}); sErr != nil {
			t.Fatalf("Send %q: %v", item, sErr)
		}
	}

	resp, err := stream.CloseAndReceive()
	if err != nil {
		t.Fatalf("CloseAndReceive: %v", err)
	}
	if resp.Count != 3 {
		t.Fatalf("want count=3, got %d", resp.Count)
	}
	if resp.Items != "x,y,z" {
		t.Fatalf("want items=x,y,z, got %q", resp.Items)
	}
}

// ============================================================
// WS Client — Bidi
// ============================================================

func TestIntegration_WSClientBidi(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewFourShapeServiceWSHandler(s, &fourShapeHandler{})

	conn := startWSClientConn(t, s)
	client := testv1.NewFourShapeServiceWSClient(conn)

	stream, err := client.Chat(t.Context())
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	inputs := []string{"hello", "world"}
	for _, text := range inputs {
		if sErr := stream.Send(&testv1.ChatMessage{Text: text}); sErr != nil {
			t.Fatalf("Send %q: %v", text, sErr)
		}
		msg, rErr := stream.Receive()
		if rErr != nil {
			t.Fatalf("Receive: %v", rErr)
		}
		if msg.Text != "echo:"+text {
			t.Fatalf("want echo:%s, got %q", text, msg.Text)
		}
	}

	if cErr := stream.CloseSend(); cErr != nil {
		t.Fatalf("CloseSend: %v", cErr)
	}

	// After CloseSend, server should close → Receive returns EOF.
	_, rErr := stream.Receive()
	if !errors.Is(rErr, io.EOF) {
		t.Fatalf("want io.EOF after CloseSend, got %v", rErr)
	}
}

// ============================================================
// WS Client — Error
// ============================================================

func TestIntegration_WSClientError(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return nil, procframe.NewError(procframe.CodePermissionDenied, "forbidden").WithRetryable()
		},
	)

	conn := startWSClientConn(t, s)

	_, err := ws.CallUnary[testv1.EchoRequest, testv1.EchoResponse](
		t.Context(), conn, "/test.v1.EchoService/Echo",
		&testv1.EchoRequest{Message: "test"},
	)
	if err == nil {
		t.Fatal("expected error")
	}

	var statusErr *procframe.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected StatusError, got %T: %v", err, err)
	}
	if statusErr.Code() != procframe.CodePermissionDenied {
		t.Fatalf("want code=%s, got %s", procframe.CodePermissionDenied, statusErr.Code())
	}
	if statusErr.Message() != "forbidden" {
		t.Fatalf("want message=forbidden, got %q", statusErr.Message())
	}
	if !statusErr.IsRetryable() {
		t.Fatal("want retryable=true")
	}
}

// ============================================================
// WS Client — Disconnect
// ============================================================

func TestIntegration_WSClientDisconnect(t *testing.T) {
	t.Parallel()

	blocked := make(chan struct{})
	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			ctx context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			close(blocked)
			<-ctx.Done()
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
	rawConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	conn := ws.NewConn(ctx, rawConn)

	// Start a blocking unary call in the background.
	errCh := make(chan error, 1)
	go func() {
		_, callErr := ws.CallUnary[testv1.EchoRequest, testv1.EchoResponse](
			ctx, conn, "/test.v1.EchoService/Echo",
			&testv1.EchoRequest{Message: "block"},
		)
		errCh <- callErr
	}()

	// Wait for handler to start blocking, then forcefully close the WS
	// connection. CloseNow avoids the close-handshake deadlock with the
	// concurrent readLoop.
	<-blocked
	rawConn.CloseNow() //nolint:errcheck // intentional forced close

	select {
	case callErr := <-errCh:
		if callErr == nil {
			t.Fatal("expected error after disconnect")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("call did not return within 5 seconds after disconnect")
	}
}

// ============================================================
// WS Client — Cancel
// ============================================================

func TestIntegration_WSClientCancel(t *testing.T) {
	t.Parallel()

	handlerDone := make(chan struct{})
	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			ctx context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			<-ctx.Done()
			close(handlerDone)
			return nil, ctx.Err()
		},
	)

	conn := startWSClientConn(t, s)

	ctx, cancel := context.WithCancel(t.Context())

	errCh := make(chan error, 1)
	go func() {
		_, callErr := ws.CallUnary[testv1.EchoRequest, testv1.EchoResponse](
			ctx, conn, "/test.v1.EchoService/Echo",
			&testv1.EchoRequest{Message: "block"},
		)
		errCh <- callErr
	}()

	// Give the call time to reach the server handler.
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case callErr := <-errCh:
		if callErr == nil {
			t.Fatal("expected error after cancel")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("call did not return within 5 seconds after cancel")
	}

	// Server handler should also have been cancelled.
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not finish within 5 seconds after cancel")
	}
}

// ============================================================
// WS Client — OptOut
// ============================================================

func TestIntegration_WSClientOptOut(t *testing.T) {
	t.Parallel()

	// CliOptionsTestService has only WsEnabled with ws.enabled = true.
	// The generated WSClient interface should only expose WsEnabled.
	// Methods without ws.enabled should be absent.

	s := ws.NewServer()
	testv1.NewCliOptionsTestServiceWSHandler(s, &cliOptionsHandler{})

	conn := startWSClientConn(t, s)
	client := testv1.NewCliOptionsTestServiceWSClient(conn)

	resp, err := client.WsEnabled(t.Context(), &testv1.PingRequest{Target: "via-ws-client"})
	if err != nil {
		t.Fatalf("WsEnabled: unexpected error: %v", err)
	}
	if resp.Result != "via-ws-client" {
		t.Fatalf("want result=via-ws-client, got %q", resp.Result)
	}

	// DefaultEnabled, ExplicitEnabled, ExplicitDisabled are not in the
	// CliOptionsTestServiceWSClient interface — compile-time verification.
}

// ============================================================
// WS Client — Empty/nil inputs
// ============================================================

func TestIntegration_WSClientEmptyProcedure(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			req *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: req.Msg.Message},
			}, nil
		},
	)

	conn := startWSClientConn(t, s)

	// Call with empty procedure: server should return not_found error.
	_, err := ws.CallUnary[testv1.EchoRequest, testv1.EchoResponse](
		t.Context(), conn, "", &testv1.EchoRequest{Message: "test"},
	)
	if err == nil {
		t.Fatal("expected error for empty procedure, got nil")
	}
	var statusErr *procframe.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected StatusError, got %T: %v", err, err)
	}
	if statusErr.Code() != procframe.CodeNotFound {
		t.Fatalf("want code=not_found, got %s", statusErr.Code())
	}
}

func TestIntegration_WSClientNilRequest(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			req *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: req.Msg.Message},
			}, nil
		},
	)

	conn := startWSClientConn(t, s)

	// Call with nil request. marshalProto should handle this gracefully
	// (proto zero value) rather than panic.
	resp, err := ws.CallUnary[testv1.EchoRequest, testv1.EchoResponse](
		t.Context(), conn, "/test.v1.EchoService/Echo", nil,
	)
	// Either an error or a valid zero-value response is acceptable.
	// A panic is NOT acceptable.
	if err != nil {
		return
	}
	if resp == nil {
		t.Fatal("got nil response without error")
	}
}

// ============================================================
// WS Client — Malicious procedure strings
// ============================================================

func TestIntegration_WSClientProcedureInjection(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			req *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: req.Msg.Message},
			}, nil
		},
	)

	conn := startWSClientConn(t, s)

	maliciousProcedures := []string{
		"/../../../etc/passwd",
		"/test.v1.EchoService/Echo\x00injected",
		"/test.v1.EchoService/Echo\r\nX-Injected: true",
		"<script>alert(1)</script>",
		"/test.v1.EchoService/Echo; DROP TABLE users;--",
	}

	for _, proc := range maliciousProcedures {
		t.Run(proc, func(t *testing.T) {
			_, err := ws.CallUnary[testv1.EchoRequest, testv1.EchoResponse](
				t.Context(), conn, proc,
				&testv1.EchoRequest{Message: "test"},
			)
			if err == nil {
				t.Fatalf("expected error for malicious procedure %q, got nil", proc)
			}
			var statusErr *procframe.StatusError
			if !errors.As(err, &statusErr) {
				t.Fatalf("expected StatusError for %q, got %T: %v", proc, err, err)
			}
			if statusErr.Code() != procframe.CodeNotFound {
				t.Fatalf("want code=not_found for %q, got %s", proc, statusErr.Code())
			}
		})
	}
}

// ============================================================
// WS Client — Concurrent sessions
// ============================================================

func TestIntegration_WSClientConcurrentSessions(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			_ context.Context,
			req *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: req.Msg.Message},
			}, nil
		},
	)

	conn := startWSClientConn(t, s)

	const concurrency = 50
	var wg sync.WaitGroup
	wg.Add(concurrency)

	errs := make(chan error, concurrency)

	for i := range concurrency {
		go func() {
			defer wg.Done()
			//nolint:gosec // G115: i is bounded by const concurrency=50
			count := int32(i)
			resp, err := ws.CallUnary[testv1.EchoRequest, testv1.EchoResponse](
				t.Context(), conn, "/test.v1.EchoService/Echo",
				&testv1.EchoRequest{Message: "concurrent", Count: count},
			)
			if err != nil {
				errs <- err
				return
			}
			if resp.Message != "concurrent" {
				errs <- errors.New("unexpected response message: " + resp.Message)
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent call error: %v", err)
	}
}

// ============================================================
// WS Client — Use after stream close
// ============================================================

func TestIntegration_WSClientUseAfterStreamClose(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewFourShapeServiceWSHandler(s, &fourShapeHandler{})

	conn := startWSClientConn(t, s)
	client := testv1.NewFourShapeServiceWSClient(conn)

	stream, err := client.Feed(t.Context(), &testv1.CollectRequest{Item: "a"})
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}

	// Drain all messages.
	for {
		_, rErr := stream.Receive()
		if errors.Is(rErr, io.EOF) {
			break
		}
		if rErr != nil {
			t.Fatalf("Receive: %v", rErr)
		}
	}

	// Receive after EOF — should not panic or hang.
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	type result struct {
		err error
	}
	ch := make(chan result, 1)
	go func() {
		_, rErr := stream.Receive()
		ch <- result{err: rErr}
	}()

	select {
	case r := <-ch:
		if r.err == nil {
			t.Fatal("expected error on Receive after EOF, got nil")
		}
	case <-ctx.Done():
		t.Fatal("Receive after EOF blocks indefinitely")
	}

	// Close after EOF — should be idempotent.
	if cErr := stream.Close(); cErr != nil {
		t.Fatalf("Close after EOF: %v", cErr)
	}
	if cErr := stream.Close(); cErr != nil {
		t.Fatalf("double Close: %v", cErr)
	}
}

// ============================================================
// WS Client — Partial send failure
// ============================================================

func TestIntegration_WSClientPartialSendFailure(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	s := ws.NewServer()
	ws.HandleUnary(s, "/test.v1.EchoService/Echo",
		func(
			ctx context.Context,
			_ *procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			close(started)
			<-ctx.Done()
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
	rawConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	conn := ws.NewConn(ctx, rawConn)

	errCh := make(chan error, 1)
	go func() {
		_, callErr := ws.CallUnary[testv1.EchoRequest, testv1.EchoResponse](
			ctx, conn, "/test.v1.EchoService/Echo",
			&testv1.EchoRequest{Message: "first"},
		)
		errCh <- callErr
	}()

	// Wait for handler to start, then kill the connection.
	<-started
	rawConn.CloseNow() //nolint:errcheck // intentional forced close

	select {
	case callErr := <-errCh:
		if callErr == nil {
			t.Fatal("expected error after connection drop, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first call did not return within 5 seconds")
	}

	// Second call on the dead connection should fail immediately.
	ctx2, cancel2 := context.WithTimeout(ctx, 2*time.Second)
	defer cancel2()

	_, err = ws.CallUnary[testv1.EchoRequest, testv1.EchoResponse](
		ctx2, conn, "/test.v1.EchoService/Echo",
		&testv1.EchoRequest{Message: "second"},
	)
	if err == nil {
		t.Fatal("expected error on dead connection, got nil")
	}
}

// ============================================================
// WS Client — Disconnect with multiple streams
// ============================================================

func TestIntegration_WSClientDisconnectMultiStream(t *testing.T) {
	t.Parallel()

	allStarted := make(chan struct{})
	var startCount int32
	var startMu sync.Mutex

	s := ws.NewServer()
	ws.HandleServerStream(s, "/test.v1.TickService/Watch",
		func(
			ctx context.Context,
			_ *procframe.Request[testv1.TickRequest],
			_ procframe.ServerStream[testv1.TickResponse],
		) error {
			startMu.Lock()
			startCount++
			if startCount == 3 {
				close(allStarted)
			}
			startMu.Unlock()
			<-ctx.Done()
			return ctx.Err()
		},
	)

	mux := http.NewServeMux()
	mux.Handle("/ws", s)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := t.Context()
	wsURL := "ws" + srv.URL[len("http"):] + "/ws"
	//nolint:bodyclose // coder/websocket manages resp.Body internally
	rawConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	conn := ws.NewConn(ctx, rawConn)

	const streamCount = 3
	errChs := make([]chan error, streamCount)
	for i := range streamCount {
		errChs[i] = make(chan error, 1)
		go func(ch chan error) {
			stream, sErr := ws.CallServerStream[testv1.TickRequest, testv1.TickResponse](
				ctx, conn, "/test.v1.TickService/Watch",
				&testv1.TickRequest{Label: "probe", Count: 1000},
			)
			if sErr != nil {
				ch <- sErr
				return
			}
			for {
				_, rErr := stream.Receive()
				if rErr != nil {
					ch <- rErr
					return
				}
			}
		}(errChs[i])
	}

	select {
	case <-allStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("not all handlers started within 5 seconds")
	}

	rawConn.CloseNow() //nolint:errcheck // intentional forced close

	// ALL streams must return errors.
	for i, ch := range errChs {
		select {
		case streamErr := <-ch:
			if streamErr == nil {
				t.Fatalf("stream %d: expected error after disconnect, got nil", i)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("stream %d: did not return within 5 seconds after disconnect", i)
		}
	}
}

// ============================================================
// WS Client — Concurrent Close and Receive
// ============================================================

func TestIntegration_WSClientConcurrentCloseAndReceive(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	ws.HandleServerStream(s, "/test.v1.TickService/Watch",
		func(
			ctx context.Context,
			_ *procframe.Request[testv1.TickRequest],
			stream procframe.ServerStream[testv1.TickResponse],
		) error {
			for i := range 100 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				if err := stream.Send(&procframe.Response[testv1.TickResponse]{
					Msg: &testv1.TickResponse{Label: "tick", Seq: int32(i)},
				}); err != nil {
					return err
				}
				time.Sleep(10 * time.Millisecond)
			}
			return nil
		},
	)

	conn := startWSClientConn(t, s)

	stream, err := ws.CallServerStream[testv1.TickRequest, testv1.TickResponse](
		t.Context(), conn, "/test.v1.TickService/Watch",
		&testv1.TickRequest{Label: "probe", Count: 100},
	)
	if err != nil {
		t.Fatalf("CallServerStream: %v", err)
	}

	for range 3 {
		_, rErr := stream.Receive()
		if rErr != nil {
			t.Fatalf("initial Receive: %v", rErr)
		}
	}

	// Race Close() and Receive() concurrently.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, rErr := stream.Receive()
			if rErr != nil {
				return
			}
		}
	}()

	time.Sleep(20 * time.Millisecond)
	if cErr := stream.Close(); cErr != nil {
		t.Fatalf("Close: %v", cErr)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: Receive did not unblock after Close")
	}
}

// ============================================================
// WS Client — Network failure mid-stream
// ============================================================

func TestIntegration_WSClientNetworkFailureMidStream(t *testing.T) {
	t.Parallel()

	sentOne := make(chan struct{})

	s := ws.NewServer()
	ws.HandleServerStream(s, "/test.v1.TickService/Watch",
		func(
			ctx context.Context,
			_ *procframe.Request[testv1.TickRequest],
			stream procframe.ServerStream[testv1.TickResponse],
		) error {
			if err := stream.Send(&procframe.Response[testv1.TickResponse]{
				Msg: &testv1.TickResponse{Label: "first", Seq: 1},
			}); err != nil {
				return err
			}
			close(sentOne)
			<-ctx.Done()
			return ctx.Err()
		},
	)

	mux := http.NewServeMux()
	mux.Handle("/ws", s)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := t.Context()
	wsURL := "ws" + srv.URL[len("http"):] + "/ws"
	//nolint:bodyclose // coder/websocket manages resp.Body internally
	rawConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	conn := ws.NewConn(ctx, rawConn)

	stream, err := ws.CallServerStream[testv1.TickRequest, testv1.TickResponse](
		ctx, conn, "/test.v1.TickService/Watch",
		&testv1.TickRequest{Label: "net-fail", Count: 1000},
	)
	if err != nil {
		t.Fatalf("CallServerStream: %v", err)
	}

	msg, err := stream.Receive()
	if err != nil {
		t.Fatalf("first Receive: %v", err)
	}
	if msg.Label != "first" || msg.Seq != 1 {
		t.Fatalf("unexpected first message: %+v", msg)
	}

	<-sentOne
	rawConn.CloseNow() //nolint:errcheck // intentional forced close

	// Next Receive must return an error, not hang.
	errCh := make(chan error, 1)
	go func() {
		_, rErr := stream.Receive()
		errCh <- rErr
	}()

	select {
	case rErr := <-errCh:
		if rErr == nil {
			t.Fatal("expected error after network failure, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Receive did not return within 5 seconds after network failure")
	}
}

// ============================================================
// WS Client — Concurrent Send and CloseSend
// ============================================================

func TestIntegration_WSClientConcurrentSendAndCloseSend(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewFourShapeServiceWSHandler(s, &fourShapeHandler{})

	conn := startWSClientConn(t, s)

	stream, err := ws.CallBidi[testv1.ChatMessage, testv1.ChatReply](
		t.Context(), conn, "/test.v1.FourShapeService/Chat",
	)
	if err != nil {
		t.Fatalf("CallBidi: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2) //nolint:mnd // Send goroutine + CloseSend goroutine

	go func() {
		defer wg.Done()
		for range 20 {
			sErr := stream.Send(&testv1.ChatMessage{Text: "race"})
			if sErr != nil {
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond)
		stream.CloseSend() //nolint:errcheck // best-effort close in race test
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: Send/CloseSend race did not complete")
	}

	// Drain remaining responses.
	for {
		_, rErr := stream.Receive()
		if rErr != nil {
			break
		}
	}
}

// ============================================================
// Adversarial helpers
// ============================================================

// assertErrorFrame checks that an outbound frame is an error with the expected code.
func assertErrorFrame(t *testing.T, frame outboundFrame, code string) {
	t.Helper()
	if frame.Error == nil {
		t.Fatalf("expected error frame, got type=%q", frame.Type)
	}
	if frame.Error.Code != code {
		t.Fatalf("want error code %q, got %q (message: %s)", code, frame.Error.Code, frame.Error.Message)
	}
}

// checkNoInternalExposure verifies an error message doesn't leak Go runtime
// internals, file paths, or stack traces.
func checkNoInternalExposure(t *testing.T, msg string) {
	t.Helper()
	sensitive := []string{
		".go:",        // Go source file references
		"goroutine ",  // stack traces
		"runtime.",    // Go runtime references
		"panic:",      // panic markers
		"/Users/",     // macOS absolute paths
		"/home/",      // Linux absolute paths
		"github.com/", // module paths in stack traces
	}
	for _, s := range sensitive {
		if strings.Contains(msg, s) {
			t.Errorf("error message leaks internal detail %q: %s", s, msg)
		}
	}
}

// ============================================================
// Integration tests — Adversarial
// ============================================================

// TestIntegration_WSErrorMessageExposure verifies that error frames returned
// by the WS server do not leak Go runtime internals, file paths, or stack
// traces to the client across all error paths.
func TestIntegration_WSErrorMessageExposure(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewEchoServiceWSHandler(s, &echoHandler{})

	// Register a handler that returns a raw Go error (not StatusError).
	ws.HandleUnary(
		s,
		"/adversarial.v1.Error/Raw",
		func(_ context.Context, _ *procframe.Request[testv1.EchoRequest]) (*procframe.Response[testv1.EchoResponse], error) {
			return nil, errors.New("internal database connection refused at 10.0.0.5:5432")
		},
	)

	conn, ctx := startWSServer(t, s)

	t.Run("invalid_protojson", func(t *testing.T) {
		sendOpen(t, ctx, conn, "exp-1", "/test.v1.EchoService/Echo", "unary")
		sendMessage(t, ctx, conn, "exp-1", json.RawMessage(`{"message": 12345}`))
		sendClose(t, ctx, conn, "exp-1")

		out := readFrame(t, ctx, conn)
		if out.Error == nil {
			// protojson accepted the value; drain close frame and skip.
			readFrame(t, ctx, conn)
			t.Skip("protojson accepted the value; no error to check")
		}
		checkNoInternalExposure(t, out.Error.Message)
	})

	t.Run("unknown_procedure", func(t *testing.T) {
		sendOpen(t, ctx, conn, "exp-2", "/evil/Procedure", "unary")
		out := readFrame(t, ctx, conn)
		assertErrorFrame(t, out, string(procframe.CodeNotFound))
		checkNoInternalExposure(t, out.Error.Message)
	})

	t.Run("shape_mismatch", func(t *testing.T) {
		sendOpen(t, ctx, conn, "exp-3", "/test.v1.EchoService/Echo", "server_stream")
		out := readFrame(t, ctx, conn)
		assertErrorFrame(t, out, string(procframe.CodeInvalidArgument))
		checkNoInternalExposure(t, out.Error.Message)
	})

	t.Run("raw_error_from_handler", func(t *testing.T) {
		payload, err := protojson.Marshal(&testv1.EchoRequest{Message: "test"})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		sendOpen(t, ctx, conn, "exp-4", "/adversarial.v1.Error/Raw", "unary")
		sendMessage(t, ctx, conn, "exp-4", payload)
		sendClose(t, ctx, conn, "exp-4")

		out := readFrame(t, ctx, conn)
		if out.Error == nil {
			t.Fatal("expected error from handler")
		}
		// Raw error message is passed through by design (toErrorFrame fallback).
		// Verify it doesn't contain Go runtime internals.
		checkNoInternalExposure(t, out.Error.Message)
		assertErrorFrame(t, out, string(procframe.CodeInternal))
	})
}

// TestIntegration_WSMaxInflightBoundary verifies the server handles
// boundary values for the maxInflight option without panic or deadlock.
func TestIntegration_WSMaxInflightBoundary(t *testing.T) {
	t.Parallel()

	t.Run("zero_rejects_all_requests", func(t *testing.T) {
		t.Parallel()

		s := ws.NewServer(ws.WithMaxInflight(0))
		testv1.NewEchoServiceWSHandler(s, &echoHandler{})
		conn, ctx := startWSServer(t, s)

		// With maxInflight=0, the semaphore is an unbuffered channel.
		// The select-with-default in handleOpen always takes the default
		// (reject), so every request gets CodeUnavailable + retryable.
		sendOpen(t, ctx, conn, "zero-1", "/test.v1.EchoService/Echo", "unary")
		out := readFrame(t, ctx, conn)
		assertErrorFrame(t, out, string(procframe.CodeUnavailable))
		if !out.Error.Retryable {
			t.Fatal("expected retryable=true for maxInflight rejection")
		}
	})

	t.Run("negative_does_not_crash_process", func(t *testing.T) {
		t.Parallel()

		// WithMaxInflight(-1) causes make(chan struct{}, -1) in serve(),
		// which panics. The panic should be caught by net/http's recovery
		// after the WS upgrade, preventing a process crash.
		s := ws.NewServer(ws.WithMaxInflight(-1))
		testv1.NewEchoServiceWSHandler(s, &echoHandler{})

		mux := http.NewServeMux()
		mux.Handle("/ws", s)
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()

		wsURL := "ws" + srv.URL[len("http"):] + "/ws"
		//nolint:bodyclose // coder/websocket manages resp.Body internally
		conn, _, err := websocket.Dial(ctx, wsURL, nil)
		if err != nil {
			// Connection failed: server panic caught. DEFENDED.
			return
		}
		defer conn.CloseNow() //nolint:errcheck // CloseNow errors are not actionable in test cleanup

		// Connection succeeded but serve() will panic.
		// Send a frame and expect disconnection.
		data, err := json.Marshal(inboundFrame{
			Type: "open", ID: "neg-1",
			Procedure: "/test.v1.EchoService/Echo", Shape: "unary",
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		_ = conn.Write(ctx, websocket.MessageText, data) //nolint:errcheck // write may fail if server panics

		readCtx, readCancel := context.WithTimeout(ctx, 1*time.Second)
		defer readCancel()
		_, _, rErr := conn.Read(readCtx)
		// Expect read error (server disconnected after panic recovery)
		// or timeout. Either is acceptable — process did not crash.
		_ = rErr
	})
}

// TestIntegration_WSClientCallAfterClose verifies that calling RPC methods
// on a client Conn after Close returns an error promptly rather than
// hanging indefinitely.
func TestIntegration_WSClientCallAfterClose(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	testv1.NewEchoServiceWSHandler(s, &echoHandler{})
	conn := startWSClientConn(t, s)

	// Close the client connection.
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Wait briefly for close to propagate through internal goroutines.
	time.Sleep(100 * time.Millisecond)

	// Call after close: must return error within timeout, not hang.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	_, err := ws.CallUnary[testv1.EchoRequest, testv1.EchoResponse](
		ctx, conn, "/test.v1.EchoService/Echo",
		&testv1.EchoRequest{Message: "after-close"},
	)
	if err == nil {
		t.Fatal("expected error for call after close, got nil")
	}
	// Key assertion: reaching this point means we didn't hang.
	// The 3s timeout was not reached.
}

// TestIntegration_WSRecvChOverflow verifies behavior when the per-session
// receive buffer is overwhelmed. The server's handleMessage uses a
// select-with-default to drop messages when recvCh is full, resulting
// in silent data loss for slow handlers.
func TestIntegration_WSRecvChOverflow(t *testing.T) {
	t.Parallel()

	const totalMessages = 100
	const recvChBufSize = 64 // matches server.go const

	startReading := make(chan struct{})
	receivedCount := make(chan int32, 1)

	s := ws.NewServer()
	ws.HandleClientStream[testv1.CollectRequest, testv1.CollectResponse](
		s,
		"/adversarial.v1.Overflow/Collect",
		func(_ context.Context, stream procframe.ClientStream[testv1.CollectRequest]) (*procframe.Response[testv1.CollectResponse], error) {
			// Block until the test signals us. This ensures the recvCh
			// buffer fills up before the handler starts consuming.
			<-startReading

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
			receivedCount <- count
			return &procframe.Response[testv1.CollectResponse]{
				Msg: &testv1.CollectResponse{Count: count},
			}, nil
		},
	)

	conn, ctx := startWSServer(t, s)

	// Open a client-stream session.
	sendOpen(t, ctx, conn, "overflow-1", "/adversarial.v1.Overflow/Collect", "client_stream")

	// Rapidly send more messages than the recvCh buffer can hold
	// while the handler is blocked.
	payload, err := protojson.Marshal(&testv1.CollectRequest{Item: "x"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for range totalMessages {
		sendMessage(t, ctx, conn, "overflow-1", payload)
	}

	// Close the send direction.
	sendClose(t, ctx, conn, "overflow-1")

	// Now unblock the handler so it reads what's buffered.
	close(startReading)

	// Wait for the handler to complete and report.
	select {
	case count := <-receivedCount:
		switch {
		case count == int32(totalMessages):
			t.Logf("all %d messages received (no overflow)", totalMessages)
		case count <= int32(recvChBufSize):
			// Confirmed: messages beyond buffer size silently dropped.
			t.Logf("received %d/%d messages; %d silently dropped (recvCh buffer overflow)",
				count, totalMessages, int32(totalMessages)-count)
		default:
			t.Logf("received %d/%d messages", count, totalMessages)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not complete within timeout — possible goroutine leak")
	}

	// Read the response frame (message + close).
	out := readFrame(t, ctx, conn)
	if out.Error != nil {
		t.Fatalf("unexpected error: %+v", out.Error)
	}
	if out.Type == "message" {
		// Drain the close frame.
		readFrame(t, ctx, conn)
	}
}
