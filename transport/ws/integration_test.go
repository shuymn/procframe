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
