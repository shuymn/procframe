package ws_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
