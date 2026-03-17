package ws_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/shuymn/procframe"
	largeprotov1 "github.com/shuymn/procframe/internal/gen/test/largeproto/v1"
	otelv1 "github.com/shuymn/procframe/internal/gen/test/otel/v1"
	ws "github.com/shuymn/procframe/transport/ws"
)

// largeProtoHandler implements LargeProtoServiceHandler.
type largeProtoHandler struct{}

func (h *largeProtoHandler) IngestSpan(
	_ context.Context,
	req *procframe.Request[largeprotov1.IngestSpanRequest],
) (*procframe.Response[largeprotov1.IngestSpanResponse], error) {
	resp := &largeprotov1.IngestSpanResponse{}
	if req.Msg.Span != nil {
		resp.SpanIdHex = hex.EncodeToString(req.Msg.Span.SpanId)
		resp.AttributeCount = int32(len(req.Msg.Span.Attributes)) //nolint:gosec // test-only
		resp.EventCount = int32(len(req.Msg.Span.Events))         //nolint:gosec // test-only
		resp.LinkCount = int32(len(req.Msg.Span.Links))           //nolint:gosec // test-only
	}
	return &procframe.Response[largeprotov1.IngestSpanResponse]{Msg: resp}, nil
}

func (h *largeProtoHandler) CollectSpans(
	_ context.Context,
	stream procframe.ClientStream[largeprotov1.CollectSpanRequest],
) (*procframe.Response[largeprotov1.CollectSpansResponse], error) {
	var totalSpans, totalEvents int32
	var traceIDs []string
	for {
		req, err := stream.Receive()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		totalSpans++
		if req.Msg.Span != nil {
			totalEvents += int32(len(req.Msg.Span.Events)) //nolint:gosec // test-only
			traceIDs = append(traceIDs, hex.EncodeToString(req.Msg.Span.TraceId))
		}
	}
	return &procframe.Response[largeprotov1.CollectSpansResponse]{
		Msg: &largeprotov1.CollectSpansResponse{
			TotalSpans:  totalSpans,
			TotalEvents: totalEvents,
			TraceIds:    strings.Join(traceIDs, ","),
		},
	}, nil
}

func (h *largeProtoHandler) WatchSpans(
	_ context.Context,
	req *procframe.Request[largeprotov1.WatchSpansRequest],
	stream procframe.ServerStream[largeprotov1.WatchSpanEvent],
) error {
	for i := range req.Msg.Limit {
		if err := stream.Send(&procframe.Response[largeprotov1.WatchSpanEvent]{
			Msg: &largeprotov1.WatchSpanEvent{
				Span: &otelv1.Span{
					Name: fmt.Sprintf("span-%d", i),
					Kind: req.Msg.KindFilter,
				},
				Source: req.Msg.ServiceName,
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (h *largeProtoHandler) StreamSpans(
	_ context.Context,
	stream procframe.BidiStream[largeprotov1.CollectSpanRequest, largeprotov1.WatchSpanEvent],
) error {
	for {
		req, err := stream.Receive()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.Send(&procframe.Response[largeprotov1.WatchSpanEvent]{
			Msg: &largeprotov1.WatchSpanEvent{
				Span:   req.Msg.Span,
				Source: "echo",
			},
		}); err != nil {
			return err
		}
	}
}

func TestIntegration_WSLargeProtoUnaryFull(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	largeprotov1.NewLargeProtoServiceWSHandler(s, &largeProtoHandler{})

	conn, ctx := startWSServer(t, s)

	payload, err := protojson.Marshal(&largeprotov1.IngestSpanRequest{
		Span: &otelv1.Span{
			TraceId:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			SpanId:     []byte{1, 1, 1, 1, 1, 1, 1, 1},
			TraceState: "vendor=opaque",
			Name:       "GET /api/v1/users",
			Kind:       otelv1.Span_SPAN_KIND_SERVER,
			Attributes: []*otelv1.KeyValue{
				{Key: "http.method", Value: &otelv1.AnyValue{Value: &otelv1.AnyValue_StringValue{StringValue: "GET"}}},
				{Key: "http.status_code", Value: &otelv1.AnyValue{Value: &otelv1.AnyValue_IntValue{IntValue: 200}}},
			},
			Events: []*otelv1.Span_Event{
				{Name: "exception", TimeUnixNano: 1500000000},
			},
			Links: []*otelv1.Span_Link{
				{
					TraceId: []byte{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3},
					SpanId:  []byte{4, 4, 4, 4, 4, 4, 4, 4},
				},
			},
			Status: &otelv1.Status{Code: otelv1.Status_STATUS_CODE_OK},
		},
		ServiceName: "user-service",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	sendOpen(t, ctx, conn, "lp-1", "/test.largeproto.v1.LargeProtoService/IngestSpan", "unary")
	sendMessage(t, ctx, conn, "lp-1", json.RawMessage(payload))
	sendClose(t, ctx, conn, "lp-1")

	out := readFrame(t, ctx, conn)
	if out.Type != "message" {
		t.Fatalf("want type=message, got %q", out.Type)
	}
	if out.Error != nil {
		t.Fatalf("unexpected error: %+v", out.Error)
	}

	var resp largeprotov1.IngestSpanResponse
	if err := protojson.Unmarshal(out.Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.SpanIdHex != "0101010101010101" {
		t.Fatalf("want spanIdHex=0101010101010101, got %q", resp.SpanIdHex)
	}
	if resp.AttributeCount != 2 {
		t.Fatalf("want attributeCount=2, got %d", resp.AttributeCount)
	}
	if resp.EventCount != 1 {
		t.Fatalf("want eventCount=1, got %d", resp.EventCount)
	}
	if resp.LinkCount != 1 {
		t.Fatalf("want linkCount=1, got %d", resp.LinkCount)
	}

	close1 := readFrame(t, ctx, conn)
	if close1.Type != "close" {
		t.Fatalf("want type=close, got %q", close1.Type)
	}
}

func TestIntegration_WSLargeProtoUnaryEmpty(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	largeprotov1.NewLargeProtoServiceWSHandler(s, &largeProtoHandler{})

	conn, ctx := startWSServer(t, s)

	payload, err := protojson.Marshal(&largeprotov1.IngestSpanRequest{
		Span: &otelv1.Span{Name: "minimal"},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	sendOpen(t, ctx, conn, "lp-2", "/test.largeproto.v1.LargeProtoService/IngestSpan", "unary")
	sendMessage(t, ctx, conn, "lp-2", json.RawMessage(payload))
	sendClose(t, ctx, conn, "lp-2")

	out := readFrame(t, ctx, conn)
	if out.Type != "message" {
		t.Fatalf("want type=message, got %q", out.Type)
	}

	var resp largeprotov1.IngestSpanResponse
	if err := protojson.Unmarshal(out.Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.AttributeCount != 0 {
		t.Fatalf("want attributeCount=0, got %d", resp.AttributeCount)
	}

	close1 := readFrame(t, ctx, conn)
	if close1.Type != "close" {
		t.Fatalf("want type=close, got %q", close1.Type)
	}
}

func TestIntegration_WSLargeProtoServerStream(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	largeprotov1.NewLargeProtoServiceWSHandler(s, &largeProtoHandler{})

	conn, ctx := startWSServer(t, s)

	payload, err := protojson.Marshal(&largeprotov1.WatchSpansRequest{
		ServiceName: "test-svc",
		Limit:       3,
		KindFilter:  otelv1.Span_SPAN_KIND_CLIENT,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	sendOpen(t, ctx, conn, "ss-1", "/test.largeproto.v1.LargeProtoService/WatchSpans", "server_stream")
	sendMessage(t, ctx, conn, "ss-1", json.RawMessage(payload))
	sendClose(t, ctx, conn, "ss-1")

	for i := range 3 {
		out := readFrame(t, ctx, conn)
		if out.Type != "message" {
			t.Fatalf("frame %d: want type=message, got %q", i, out.Type)
		}
		var event largeprotov1.WatchSpanEvent
		if err := protojson.Unmarshal(out.Payload, &event); err != nil {
			t.Fatalf("frame %d: unmarshal: %v", i, err)
		}
		if event.Source != "test-svc" {
			t.Fatalf("frame %d: want source=test-svc, got %q", i, event.Source)
		}
	}

	eos := readFrame(t, ctx, conn)
	if eos.Type != "close" {
		t.Fatalf("want type=close, got %q", eos.Type)
	}
}

func TestIntegration_WSLargeProtoClientStream(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	largeprotov1.NewLargeProtoServiceWSHandler(s, &largeProtoHandler{})

	conn, ctx := startWSServer(t, s)

	sendOpen(t, ctx, conn, "cs-1", "/test.largeproto.v1.LargeProtoService/CollectSpans", "client_stream")

	spans := []struct {
		name    string
		traceID []byte
	}{
		{"span-a", []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{"span-b", []byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
	}
	for _, s := range spans {
		payload, err := protojson.Marshal(&largeprotov1.CollectSpanRequest{
			Span: &otelv1.Span{Name: s.name, TraceId: s.traceID},
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		sendMessage(t, ctx, conn, "cs-1", json.RawMessage(payload))
	}
	sendClose(t, ctx, conn, "cs-1")

	out := readFrame(t, ctx, conn)
	if out.Type != "message" {
		t.Fatalf("want type=message, got %q", out.Type)
	}

	var resp largeprotov1.CollectSpansResponse
	if err := protojson.Unmarshal(out.Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.TotalSpans != 2 {
		t.Fatalf("want totalSpans=2, got %d", resp.TotalSpans)
	}

	close1 := readFrame(t, ctx, conn)
	if close1.Type != "close" {
		t.Fatalf("want type=close, got %q", close1.Type)
	}
}

func TestIntegration_WSLargeProtoBidi(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	largeprotov1.NewLargeProtoServiceWSHandler(s, &largeProtoHandler{})

	conn, ctx := startWSServer(t, s)

	sendOpen(t, ctx, conn, "bi-1", "/test.largeproto.v1.LargeProtoService/StreamSpans", "bidi")

	inputs := []string{"bidi-1", "bidi-2"}
	for _, name := range inputs {
		payload, err := protojson.Marshal(&largeprotov1.CollectSpanRequest{
			Span: &otelv1.Span{Name: name},
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		sendMessage(t, ctx, conn, "bi-1", json.RawMessage(payload))

		out := readFrame(t, ctx, conn)
		if out.Type != "message" {
			t.Fatalf("want type=message, got %q", out.Type)
		}
		var event largeprotov1.WatchSpanEvent
		if err := protojson.Unmarshal(out.Payload, &event); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if event.Source != "echo" {
			t.Fatalf("want source=echo, got %q", event.Source)
		}
		if event.Span.Name != name {
			t.Fatalf("want name=%q, got %q", name, event.Span.Name)
		}
	}

	sendClose(t, ctx, conn, "bi-1")

	close1 := readFrame(t, ctx, conn)
	if close1.Type != "close" {
		t.Fatalf("want type=close, got %q", close1.Type)
	}
}

func TestIntegration_WSLargeProtoLargePayload(t *testing.T) {
	t.Parallel()

	s := ws.NewServer()
	largeprotov1.NewLargeProtoServiceWSHandler(s, &largeProtoHandler{})

	conn, ctx := startWSServer(t, s)

	attrs := make([]*otelv1.KeyValue, 100)
	for i := range attrs {
		attrs[i] = &otelv1.KeyValue{
			Key:   fmt.Sprintf("attr-%d", i),
			Value: &otelv1.AnyValue{Value: &otelv1.AnyValue_StringValue{StringValue: fmt.Sprintf("val-%d", i)}},
		}
	}

	payload, err := protojson.Marshal(&largeprotov1.IngestSpanRequest{
		Span: &otelv1.Span{
			Name:       "large-span",
			TraceId:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			SpanId:     []byte{1, 1, 1, 1, 1, 1, 1, 1},
			Attributes: attrs,
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	sendOpen(t, ctx, conn, "big-1", "/test.largeproto.v1.LargeProtoService/IngestSpan", "unary")
	sendMessage(t, ctx, conn, "big-1", json.RawMessage(payload))
	sendClose(t, ctx, conn, "big-1")

	out := readFrame(t, ctx, conn)
	if out.Type != "message" {
		t.Fatalf("want type=message, got %q", out.Type)
	}

	var resp largeprotov1.IngestSpanResponse
	if err := protojson.Unmarshal(out.Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.AttributeCount != 100 {
		t.Fatalf("want attributeCount=100, got %d", resp.AttributeCount)
	}

	close1 := readFrame(t, ctx, conn)
	if close1.Type != "close" {
		t.Fatalf("want type=close, got %q", close1.Type)
	}
}
