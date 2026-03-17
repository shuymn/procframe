package cli_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/shuymn/procframe"
	largeprotov1 "github.com/shuymn/procframe/internal/gen/test/largeproto/v1"
	otelv1 "github.com/shuymn/procframe/internal/gen/test/otel/v1"
	"github.com/shuymn/procframe/transport/cli"
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
		name := ""
		if req.Msg.Span != nil {
			name = req.Msg.Span.Name
		}
		if err := stream.Send(&procframe.Response[largeprotov1.WatchSpanEvent]{
			Msg: &largeprotov1.WatchSpanEvent{
				Span:   req.Msg.Span,
				Source: "echo",
			},
		}); err != nil {
			_ = name
			return err
		}
	}
}

// makeFullSpanJSON returns a JSON string for a fully populated Span.
func makeFullSpanJSON() string {
	return `{
		"span": {
			"traceId": "AAAAAAAAAAAAAAAAAAAAAA==",
			"spanId": "AQEBAQEBAQE=",
			"traceState": "vendor=opaque",
			"parentSpanId": "AgICAgICAgI=",
			"name": "GET /api/v1/users",
			"kind": "SPAN_KIND_SERVER",
			"startTimeUnixNano": "1000000000",
			"endTimeUnixNano": "2000000000",
			"attributes": [
				{"key": "http.method", "value": {"stringValue": "GET"}},
				{"key": "http.status_code", "value": {"intValue": "200"}},
				{"key": "service.active", "value": {"boolValue": true}},
				{"key": "latency.ms", "value": {"doubleValue": 42.5}}
			],
			"droppedAttributesCount": 1,
			"events": [
				{
					"timeUnixNano": "1500000000",
					"name": "exception",
					"attributes": [
						{"key": "exception.type", "value": {"stringValue": "RuntimeError"}}
					],
					"droppedAttributesCount": 0
				}
			],
			"droppedEventsCount": 0,
			"links": [
				{
					"traceId": "AwMDAwMDAwMDAwMDAwMDAw==",
					"spanId": "BAQEBAQEBAQ=",
					"traceState": "",
					"attributes": [
						{"key": "link.type", "value": {"stringValue": "parent"}}
					],
					"droppedAttributesCount": 0
				}
			],
			"droppedLinksCount": 0,
			"status": {
				"message": "",
				"code": "STATUS_CODE_OK"
			}
		},
		"serviceName": "user-service",
		"dryRun": false
	}`
}

func TestIntegration_LargeProto_UnaryJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := largeprotov1.NewLargeProtoServiceCLIRunner(
		&largeProtoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"--json", makeFullSpanJSON(),
		"large", "ingest",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, `"spanIdHex"`) {
		t.Fatalf("want spanIdHex in output, got:\n%s", out)
	}
	if !strings.Contains(out, `"attributeCount"`) {
		t.Fatalf("want attributeCount in output, got:\n%s", out)
	}
}

func TestIntegration_LargeProto_UnaryMinimal(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := largeprotov1.NewLargeProtoServiceCLIRunner(
		&largeProtoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"--json",
		`{"span":{"name":"minimal","traceId":"AAAAAAAAAAAAAAAAAAAAAA==","spanId":"AQEBAQEBAQE="},"serviceName":"test"}`,
		"large",
		"ingest",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	// Span has spanId so spanIdHex should be present
	if !strings.Contains(out, `"spanIdHex"`) {
		t.Fatalf("want spanIdHex in output, got:\n%s", out)
	}
}

func TestIntegration_LargeProto_UnaryEmptyNested(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := largeprotov1.NewLargeProtoServiceCLIRunner(
		&largeProtoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"--json", `{"span":{}}`,
		"large", "ingest",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	// All counts should be zero (default)
	var resp map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		// protojson output may be multiline; try reading first JSON object
		dec := json.NewDecoder(strings.NewReader(out))
		if err := dec.Decode(&resp); err != nil {
			t.Fatalf("invalid JSON output: %v\nraw: %s", err, out)
		}
	}
}

func TestIntegration_LargeProto_ServerStreamFlags(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := largeprotov1.NewLargeProtoServiceCLIRunner(
		&largeProtoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"large", "watch",
		"--service-name", "my-service",
		"--limit", "3",
		"--since-unix-nano", "1000000000",
		"--include-events",
		"--kind-filter", "server",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if strings.Count(out, `"my-service"`) != 3 {
		t.Fatalf("want 3 events with source=my-service, got:\n%s", out)
	}
	// SpanKind is serialized as its protojson name
	if strings.Count(out, `SERVER`) != 3 {
		t.Fatalf("want 3 events with SERVER kind, got:\n%s", out)
	}
}

func TestIntegration_LargeProto_ClientStreamNDJSON(t *testing.T) {
	t.Parallel()

	stdin := strings.NewReader(
		`{"span":{"name":"s1","traceId":"AAAAAAAAAAAAAAAAAAAAAA==","spanId":"AQEBAQEBAQE=","events":[{"name":"e1"}]}}` + "\n" +
			`{"span":{"name":"s2","traceId":"AgICAgICAgICAgICAgICAg==","spanId":"AwMDAwMDAwM="}}` + "\n",
	)

	var stdout bytes.Buffer
	runner := largeprotov1.NewLargeProtoServiceCLIRunner(
		&largeProtoHandler{},
		cli.WithStdin(stdin),
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"--output", "json",
		"large", "collect",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout.String())
	}
}

func TestIntegration_LargeProto_BidiNDJSON(t *testing.T) {
	t.Parallel()

	stdin := strings.NewReader(
		`{"span":{"name":"bidi-1"}}` + "\n" +
			`{"span":{"name":"bidi-2"}}` + "\n",
	)

	var stdout bytes.Buffer
	runner := largeprotov1.NewLargeProtoServiceCLIRunner(
		&largeProtoHandler{},
		cli.WithStdin(stdin),
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"--output", "json",
		"large", "stream",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 NDJSON lines, got %d:\n%s", len(lines), stdout.String())
	}
	for _, line := range lines {
		if !strings.Contains(line, `"echo"`) {
			t.Fatalf("want source=echo in each line, got: %s", line)
		}
	}
}

func TestIntegration_LargeProto_LargePayload(t *testing.T) {
	t.Parallel()

	// Build a span with 100 attributes
	attrs := make([]string, 100)
	for i := range attrs {
		attrs[i] = fmt.Sprintf(`{"key":"attr-%d","value":{"stringValue":"val-%d"}}`, i, i)
	}
	spanJSON := fmt.Sprintf(
		`{"span":{"name":"large","traceId":"AAAAAAAAAAAAAAAAAAAAAA==","spanId":"AQEBAQEBAQE=","attributes":[%s]}}`,
		strings.Join(attrs, ","),
	)

	var stdout bytes.Buffer
	runner := largeprotov1.NewLargeProtoServiceCLIRunner(
		&largeProtoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"--json", spanJSON,
		"large", "ingest",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	// Verify the handler counted all 100 attributes
	if !strings.Contains(out, `100`) {
		t.Fatalf("want attributeCount containing 100, got:\n%s", out)
	}
}

func TestIntegration_LargeProto_Schema(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := largeprotov1.NewLargeProtoServiceCLIRunner(
		&largeProtoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"schema", "large", "watch"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var info cli.CommandInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout.String())
	}
	if info.Command != "large watch" {
		t.Fatalf("want command=%q, got %q", "large watch", info.Command)
	}
	if info.Procedure != "/test.largeproto.v1.LargeProtoService/WatchSpans" {
		t.Fatalf("want procedure, got %q", info.Procedure)
	}
	// Verify flag types: service_name(string), limit(int32), since_unix_nano(int64),
	// include_events(bool), kind_filter(enum)
	wantFlags := map[string]string{
		"service_name":    "string",
		"limit":           "int32",
		"since_unix_nano": "int64",
		"include_events":  "bool",
		"kind_filter":     "enum",
	}
	for _, f := range info.Flags {
		if want, ok := wantFlags[f.Name]; ok {
			if f.Type != want {
				t.Errorf("flag %q: want type %q, got %q", f.Name, want, f.Type)
			}
			delete(wantFlags, f.Name)
		}
	}
	for name := range wantFlags {
		t.Errorf("missing flag %q in schema", name)
	}
}

func TestIntegration_LargeProto_HelpShowsDescriptions(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	runner := largeprotov1.NewLargeProtoServiceCLIRunner(
		&largeProtoHandler{},
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&stderr),
	)

	err := runner.Run(t.Context(), []string{"large", "watch", "--help"})
	if err != nil {
		t.Fatalf("--help should not error, got: %v", err)
	}

	out := stderr.String()
	for _, want := range []string{
		"Filter by service name",
		"Maximum spans to return",
		"Start time filter in unix nanos",
		"Include span events in output",
		"Filter by span kind",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("want %q in help output, got:\n%s", want, out)
		}
	}
}
