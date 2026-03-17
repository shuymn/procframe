package connect_test

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connectrpc "connectrpc.com/connect"

	"github.com/shuymn/procframe"
	largeprotov1 "github.com/shuymn/procframe/internal/gen/test/largeproto/v1"
	otelv1 "github.com/shuymn/procframe/internal/gen/test/otel/v1"
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

func makeFullSpan() *otelv1.Span {
	return &otelv1.Span{
		TraceId:           []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
		SpanId:            []byte{1, 1, 1, 1, 1, 1, 1, 1},
		TraceState:        "vendor=opaque",
		ParentSpanId:      []byte{2, 2, 2, 2, 2, 2, 2, 2},
		Name:              "GET /api/v1/users",
		Kind:              otelv1.Span_SPAN_KIND_SERVER,
		StartTimeUnixNano: 1000000000,
		EndTimeUnixNano:   2000000000,
		Attributes: []*otelv1.KeyValue{
			{Key: "http.method", Value: &otelv1.AnyValue{Value: &otelv1.AnyValue_StringValue{StringValue: "GET"}}},
			{Key: "http.status_code", Value: &otelv1.AnyValue{Value: &otelv1.AnyValue_IntValue{IntValue: 200}}},
			{Key: "service.active", Value: &otelv1.AnyValue{Value: &otelv1.AnyValue_BoolValue{BoolValue: true}}},
			{Key: "latency.ms", Value: &otelv1.AnyValue{Value: &otelv1.AnyValue_DoubleValue{DoubleValue: 42.5}}},
		},
		DroppedAttributesCount: 1,
		Events: []*otelv1.Span_Event{
			{
				TimeUnixNano: 1500000000,
				Name:         "exception",
				Attributes: []*otelv1.KeyValue{
					{
						Key:   "exception.type",
						Value: &otelv1.AnyValue{Value: &otelv1.AnyValue_StringValue{StringValue: "RuntimeError"}},
					},
				},
			},
		},
		Links: []*otelv1.Span_Link{
			{
				TraceId: []byte{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3},
				SpanId:  []byte{4, 4, 4, 4, 4, 4, 4, 4},
				Attributes: []*otelv1.KeyValue{
					{
						Key:   "link.type",
						Value: &otelv1.AnyValue{Value: &otelv1.AnyValue_StringValue{StringValue: "parent"}},
					},
				},
			},
		},
		Status: &otelv1.Status{
			Code: otelv1.Status_STATUS_CODE_OK,
		},
	}
}

func TestIntegration_ConnectLargeProtoUnaryFull(t *testing.T) {
	t.Parallel()

	h := &largeProtoHandler{}
	path, handler := largeprotov1.NewLargeProtoServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := connectrpc.NewClient[largeprotov1.IngestSpanRequest, largeprotov1.IngestSpanResponse](
		srv.Client(),
		srv.URL+"/test.largeproto.v1.LargeProtoService/IngestSpan",
	)

	resp, err := client.CallUnary(t.Context(), connectrpc.NewRequest(&largeprotov1.IngestSpanRequest{
		Span:        makeFullSpan(),
		ServiceName: "user-service",
		DryRun:      false,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.SpanIdHex != "0101010101010101" {
		t.Fatalf("want spanIdHex=0101010101010101, got %q", resp.Msg.SpanIdHex)
	}
	if resp.Msg.AttributeCount != 4 {
		t.Fatalf("want attributeCount=4, got %d", resp.Msg.AttributeCount)
	}
	if resp.Msg.EventCount != 1 {
		t.Fatalf("want eventCount=1, got %d", resp.Msg.EventCount)
	}
	if resp.Msg.LinkCount != 1 {
		t.Fatalf("want linkCount=1, got %d", resp.Msg.LinkCount)
	}
}

func TestIntegration_ConnectLargeProtoUnaryEmpty(t *testing.T) {
	t.Parallel()

	h := &largeProtoHandler{}
	path, handler := largeprotov1.NewLargeProtoServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := connectrpc.NewClient[largeprotov1.IngestSpanRequest, largeprotov1.IngestSpanResponse](
		srv.Client(),
		srv.URL+"/test.largeproto.v1.LargeProtoService/IngestSpan",
	)

	resp, err := client.CallUnary(t.Context(), connectrpc.NewRequest(&largeprotov1.IngestSpanRequest{
		Span: &otelv1.Span{Name: "minimal"},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.AttributeCount != 0 {
		t.Fatalf("want attributeCount=0, got %d", resp.Msg.AttributeCount)
	}
}

func TestIntegration_ConnectLargeProtoServerStream(t *testing.T) {
	t.Parallel()

	h := &largeProtoHandler{}
	path, handler := largeprotov1.NewLargeProtoServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := connectrpc.NewClient[largeprotov1.WatchSpansRequest, largeprotov1.WatchSpanEvent](
		srv.Client(),
		srv.URL+"/test.largeproto.v1.LargeProtoService/WatchSpans",
	)

	stream, err := client.CallServerStream(t.Context(), connectrpc.NewRequest(&largeprotov1.WatchSpansRequest{
		ServiceName: "test-svc",
		Limit:       3,
		KindFilter:  otelv1.Span_SPAN_KIND_CLIENT,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var msgs []*largeprotov1.WatchSpanEvent
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
		if msg.Source != "test-svc" {
			t.Fatalf("msg[%d]: want source=test-svc, got %q", i, msg.Source)
		}
		if msg.Span.Kind != otelv1.Span_SPAN_KIND_CLIENT {
			t.Fatalf("msg[%d]: want SPAN_KIND_CLIENT, got %v", i, msg.Span.Kind)
		}
	}
}

func TestIntegration_ConnectLargeProtoClientStream(t *testing.T) {
	t.Parallel()

	h := &largeProtoHandler{}
	path, handler := largeprotov1.NewLargeProtoServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := connectrpc.NewClient[largeprotov1.CollectSpanRequest, largeprotov1.CollectSpansResponse](
		srv.Client(),
		srv.URL+"/test.largeproto.v1.LargeProtoService/CollectSpans",
	)

	stream := client.CallClientStream(t.Context())
	spans := []*otelv1.Span{
		{
			Name:    "span-a",
			TraceId: []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			Events:  []*otelv1.Span_Event{{Name: "e1"}},
		},
		{Name: "span-b", TraceId: []byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
	}
	for _, s := range spans {
		if err := stream.Send(&largeprotov1.CollectSpanRequest{Span: s}); err != nil {
			t.Fatalf("send: %v", err)
		}
	}

	resp, err := stream.CloseAndReceive()
	if err != nil {
		t.Fatalf("CloseAndReceive: %v", err)
	}
	if resp.Msg.TotalSpans != 2 {
		t.Fatalf("want totalSpans=2, got %d", resp.Msg.TotalSpans)
	}
	if resp.Msg.TotalEvents != 1 {
		t.Fatalf("want totalEvents=1, got %d", resp.Msg.TotalEvents)
	}
}

func TestIntegration_ConnectLargeProtoBidiStream(t *testing.T) {
	t.Parallel()

	h := &largeProtoHandler{}
	path, handler := largeprotov1.NewLargeProtoServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	client := connectrpc.NewClient[largeprotov1.CollectSpanRequest, largeprotov1.WatchSpanEvent](
		srv.Client(),
		srv.URL+"/test.largeproto.v1.LargeProtoService/StreamSpans",
		connectrpc.WithGRPC(),
	)

	stream := client.CallBidiStream(t.Context())

	inputs := []string{"bidi-1", "bidi-2"}
	for _, name := range inputs {
		if err := stream.Send(&largeprotov1.CollectSpanRequest{
			Span: &otelv1.Span{Name: name},
		}); err != nil {
			t.Fatalf("send %q: %v", name, err)
		}
	}
	if err := stream.CloseRequest(); err != nil {
		t.Fatalf("CloseRequest: %v", err)
	}

	var replies []*largeprotov1.WatchSpanEvent
	for {
		msg, err := stream.Receive()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Receive: %v", err)
		}
		replies = append(replies, msg)
	}
	if err := stream.CloseResponse(); err != nil {
		t.Fatalf("CloseResponse: %v", err)
	}

	if len(replies) != 2 {
		t.Fatalf("want 2 replies, got %d", len(replies))
	}
	for i, r := range replies {
		if r.Source != "echo" {
			t.Fatalf("reply[%d]: want source=echo, got %q", i, r.Source)
		}
		if r.Span.Name != inputs[i] {
			t.Fatalf("reply[%d]: want name=%q, got %q", i, inputs[i], r.Span.Name)
		}
	}
}

func TestIntegration_ConnectLargeProtoLargePayload(t *testing.T) {
	t.Parallel()

	h := &largeProtoHandler{}
	path, handler := largeprotov1.NewLargeProtoServiceConnectHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Build a span with 100 attributes
	attrs := make([]*otelv1.KeyValue, 100)
	for i := range attrs {
		attrs[i] = &otelv1.KeyValue{
			Key:   fmt.Sprintf("attr-%d", i),
			Value: &otelv1.AnyValue{Value: &otelv1.AnyValue_StringValue{StringValue: fmt.Sprintf("val-%d", i)}},
		}
	}

	client := connectrpc.NewClient[largeprotov1.IngestSpanRequest, largeprotov1.IngestSpanResponse](
		srv.Client(),
		srv.URL+"/test.largeproto.v1.LargeProtoService/IngestSpan",
	)

	resp, err := client.CallUnary(t.Context(), connectrpc.NewRequest(&largeprotov1.IngestSpanRequest{
		Span: &otelv1.Span{
			Name:       "large-span",
			TraceId:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			SpanId:     []byte{1, 1, 1, 1, 1, 1, 1, 1},
			Attributes: attrs,
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.AttributeCount != 100 {
		t.Fatalf("want attributeCount=100, got %d", resp.Msg.AttributeCount)
	}
}
