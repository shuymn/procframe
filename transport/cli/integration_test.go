package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shuymn/procframe"
	testv1 "github.com/shuymn/procframe/gen/test/v1"
	"github.com/shuymn/procframe/transport/cli"
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

func TestIntegration_EchoUnary(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	runner := testv1.NewEchoServiceCLIRunner(
		&echoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&stderr),
	)

	err := runner.Run(t.Context(), []string{"echo", "run", "--message", "hello", "--count", "3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, `"hello"`) {
		t.Fatalf("want message in output, got:\n%s", out)
	}
	if !strings.Contains(out, `"count"`) {
		t.Fatalf("want count in output, got:\n%s", out)
	}
}

func TestIntegration_EchoWithBoolFlag(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := testv1.NewEchoServiceCLIRunner(
		&echoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"echo", "run", "--message", "hello", "--uppercase"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout.String(), `"HELLO"`) {
		t.Fatalf("want uppercased message, got:\n%s", stdout.String())
	}
}

func TestIntegration_EchoHelp(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	runner := testv1.NewEchoServiceCLIRunner(
		&echoHandler{},
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&stderr),
	)

	err := runner.Run(t.Context(), []string{"echo", "--help"})
	if err != nil {
		t.Fatalf("--help should not error, got: %v", err)
	}

	out := stderr.String()
	if !strings.Contains(out, "run") {
		t.Fatalf("want help to list 'run' command, got:\n%s", out)
	}
}

func TestIntegration_UnknownCommand(t *testing.T) {
	t.Parallel()

	runner := testv1.NewEchoServiceCLIRunner(
		&echoHandler{},
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"echo", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}

// errorHandler returns an error from the handler.
type errorHandler struct{}

func (h *errorHandler) Echo(
	_ context.Context,
	_ *procframe.Request[testv1.EchoRequest],
) (*procframe.Response[testv1.EchoResponse], error) {
	return nil, &procframe.Error{
		Code:    procframe.CodeNotFound,
		Message: "resource not found",
	}
}

func TestIntegration_HandlerError(t *testing.T) {
	t.Parallel()

	runner := testv1.NewEchoServiceCLIRunner(
		&errorHandler{},
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"echo", "run", "--message", "test"})
	if err == nil {
		t.Fatal("expected error from handler")
	}

	var pfErr *procframe.Error
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected procframe.Error, got %T: %v", err, err)
	}
	if pfErr.Code != procframe.CodeNotFound {
		t.Fatalf("want CodeNotFound, got %q", pfErr.Code)
	}

	exitCode := cli.ExitCode(pfErr.Code)
	if exitCode != 3 {
		t.Fatalf("want exit code 3, got %d", exitCode)
	}
}

// prHandler is a test handler for PRService.
type prHandler struct {
	lastReq *testv1.PRListRequest
}

func (h *prHandler) List(
	_ context.Context,
	req *procframe.Request[testv1.PRListRequest],
) (*procframe.Response[testv1.PRListResponse], error) {
	h.lastReq = req.Msg
	return &procframe.Response[testv1.PRListResponse]{
		Msg: &testv1.PRListResponse{
			Items: []string{"pr-1", "pr-2"},
		},
	}, nil
}

func TestIntegration_PRServiceNestedGroups(t *testing.T) {
	t.Parallel()

	h := &prHandler{}
	var stdout bytes.Buffer
	runner := testv1.NewPRServiceCLIRunner(
		h,
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"repo", "pr", "list", "--limit", "10"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h.lastReq == nil {
		t.Fatal("handler was not called")
	}
	if h.lastReq.Limit != 10 {
		t.Fatalf("want limit=10, got %d", h.lastReq.Limit)
	}

	out := stdout.String()
	if !strings.Contains(out, "pr-1") || !strings.Contains(out, "pr-2") {
		t.Fatalf("want items in output, got:\n%s", out)
	}
}

func TestIntegration_PRServiceBindInto(t *testing.T) {
	t.Parallel()

	h := &prHandler{}
	runner := testv1.NewPRServiceCLIRunner(
		h,
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"repo", "pr", "--state", "open", "list", "--limit", "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h.lastReq == nil {
		t.Fatal("handler was not called")
	}
	if h.lastReq.Pr == nil {
		t.Fatal("want Pr field injected via bind_into, got nil")
	}
	if h.lastReq.Pr.State != testv1.PRState_PR_STATE_OPEN {
		t.Fatalf("want Pr.State=PR_STATE_OPEN, got %v", h.lastReq.Pr.State)
	}
	if h.lastReq.Limit != 5 {
		t.Fatalf("want Limit=5, got %d", h.lastReq.Limit)
	}
}

func TestIntegration_PRServiceHelp(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	runner := testv1.NewPRServiceCLIRunner(
		&prHandler{},
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&stderr),
	)

	err := runner.Run(t.Context(), []string{"repo", "pr", "--help"})
	if err != nil {
		t.Fatalf("--help should not error, got: %v", err)
	}

	out := stderr.String()
	if !strings.Contains(out, "list") {
		t.Fatalf("want help to list 'list' command, got:\n%s", out)
	}
}

// --- AC1: server-stream handler ---

// tickHandler is a test handler for TickService.
type tickHandler struct {
	watchErr error
}

func (h *tickHandler) Watch(
	_ context.Context,
	req *procframe.Request[testv1.TickRequest],
	stream procframe.ServerStream[testv1.TickResponse],
) error {
	if h.watchErr != nil {
		return h.watchErr
	}
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

func TestIntegration_StreamingHandler(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := testv1.NewTickServiceCLIRunner(
		&tickHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"tick", "watch", "--label", "ping", "--count", "3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if strings.Count(out, `"ping"`) != 3 {
		t.Fatalf("want 3 chunks with label, got:\n%s", out)
	}
}

// --- AC2: --json input ---

func TestIntegration_JSONInput(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := testv1.NewEchoServiceCLIRunner(
		&echoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"--json", `{"message":"from-json","count":42}`,
		"echo", "run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout.String(), "from-json") {
		t.Fatalf("want message from JSON, got:\n%s", stdout.String())
	}
}

// --- AC3: --json + flags conflict ---

func TestIntegration_JSONAndFlagsConflict(t *testing.T) {
	t.Parallel()

	runner := testv1.NewEchoServiceCLIRunner(
		&echoHandler{},
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"--json", `{"message":"hi"}`,
		"echo", "run", "--count", "1",
	})
	if err == nil {
		t.Fatal("expected error for --json + flags")
	}
	var pfErr *procframe.Error
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected procframe.Error, got %T: %v", err, err)
	}
	if pfErr.Code != procframe.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %q", pfErr.Code)
	}
	if !strings.Contains(err.Error(), "--json cannot be combined with flags") {
		t.Fatalf("want conflict error, got: %v", err)
	}
}

// --- AC4: schema command ---

func TestIntegration_Schema(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := testv1.NewEchoServiceCLIRunner(
		&echoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"schema"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "/test.v1.EchoService/Echo") {
		t.Fatalf("want procedure path in schema, got:\n%s", out)
	}
	if !strings.Contains(out, "test.v1.EchoRequest") {
		t.Fatalf("want request type in schema, got:\n%s", out)
	}
}

func TestIntegration_SchemaSpecificProcedure(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := testv1.NewEchoServiceCLIRunner(
		&echoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"schema", "/test.v1.EchoService/Echo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var info cli.SchemaInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout.String())
	}
	if info.Procedure != "/test.v1.EchoService/Echo" {
		t.Fatalf("want procedure, got %q", info.Procedure)
	}
	if len(info.Request.Fields) != 3 {
		t.Fatalf("want 3 request fields, got %d", len(info.Request.Fields))
	}
}

func TestIntegration_SchemaStreamingFlag(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := testv1.NewTickServiceCLIRunner(
		&tickHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"schema", "/test.v1.TickService/Watch"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var info cli.SchemaInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !info.Streaming {
		t.Fatal("want streaming=true for Watch")
	}
}

// --- AC5: --output json (unary) ---

func TestIntegration_OutputJSON_Unary(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := testv1.NewEchoServiceCLIRunner(
		&echoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"--output", "json",
		"echo", "run", "--message", "compact",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := strings.TrimSpace(stdout.String())
	// Compact JSON should be a single line
	if strings.Count(out, "\n") > 0 {
		t.Fatalf("want compact single-line JSON, got:\n%s", out)
	}
	if !strings.Contains(out, `"compact"`) {
		t.Fatalf("want message in output, got:\n%s", out)
	}
}

// --- AC6: --output json + server-stream → NDJSON ---

func TestIntegration_OutputJSON_Stream_NDJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := testv1.NewTickServiceCLIRunner(
		&tickHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"--output", "json",
		"tick", "watch", "--label", "ndjson", "--count", "3",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 NDJSON lines, got %d:\n%s", len(lines), stdout.String())
	}
	for _, line := range lines {
		if !strings.Contains(line, `"ndjson"`) {
			t.Fatalf("want label in each line, got: %s", line)
		}
		// Each line should be valid JSON
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid JSON line: %v\nline: %s", err, line)
		}
	}
}

// --- AC7: exit code mapping ---

func TestIntegration_ExitCode(t *testing.T) {
	t.Parallel()

	runner := testv1.NewEchoServiceCLIRunner(
		&errorHandler{},
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"echo", "run", "--message", "test"})
	if err == nil {
		t.Fatal("expected error")
	}

	var pfErr *procframe.Error
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected procframe.Error, got %T", err)
	}

	code := cli.ExitCode(pfErr.Code)
	if code != 3 { // CodeNotFound → 3
		t.Fatalf("want exit code 3, got %d", code)
	}
}

func TestIntegration_StreamingExitCode(t *testing.T) {
	t.Parallel()

	runner := testv1.NewTickServiceCLIRunner(
		&tickHandler{watchErr: &procframe.Error{
			Code:    procframe.CodeUnavailable,
			Message: "service down",
		}},
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"tick", "watch", "--label", "x", "--count", "1"})
	if err == nil {
		t.Fatal("expected error")
	}

	var pfErr *procframe.Error
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected procframe.Error, got %T", err)
	}
	if cli.ExitCode(pfErr.Code) != 8 { // CodeUnavailable → 8
		t.Fatalf("want exit code 8, got %d", cli.ExitCode(pfErr.Code))
	}
}

// --- AC8: structured error with --output json ---

func TestIntegration_StructuredError_OutputJSON(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	runner := testv1.NewEchoServiceCLIRunner(
		&errorHandler{},
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&stderr),
	)

	err := runner.Run(t.Context(), []string{
		"--output", "json",
		"echo", "run", "--message", "test",
	})
	if err == nil {
		t.Fatal("expected error")
	}

	// Verify structured error on stderr
	var se struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if jsonErr := json.Unmarshal(stderr.Bytes(), &se); jsonErr != nil {
		t.Fatalf("invalid JSON on stderr: %v\nraw: %s", jsonErr, stderr.String())
	}
	if se.Error.Code != "not_found" {
		t.Fatalf("want code=not_found, got %q", se.Error.Code)
	}
	if se.Error.Message != "resource not found" {
		t.Fatalf("want message, got %q", se.Error.Message)
	}
}

func TestIntegration_StructuredError_NotWrittenWithoutOutputJSON(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	runner := testv1.NewEchoServiceCLIRunner(
		&errorHandler{},
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&stderr),
	)

	err := runner.Run(t.Context(), []string{"echo", "run", "--message", "test"})
	if err == nil {
		t.Fatal("expected error")
	}

	// Without --output json, stderr should be empty (no structured error)
	if stderr.Len() != 0 {
		t.Fatalf("want empty stderr without --output json, got:\n%s", stderr.String())
	}
}

// --- nil response handling ---

// nilResponseHandler returns (nil, nil) from the handler.
type nilResponseHandler struct{}

func (h *nilResponseHandler) Echo(
	_ context.Context,
	_ *procframe.Request[testv1.EchoRequest],
) (*procframe.Response[testv1.EchoResponse], error) {
	return nil, nil
}

func TestIntegration_NilResponse(t *testing.T) {
	t.Parallel()

	runner := testv1.NewEchoServiceCLIRunner(
		&nilResponseHandler{},
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"echo", "run", "--message", "test"})
	if err == nil {
		t.Fatal("expected error for nil response")
	}
	var pfErr *procframe.Error
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected procframe.Error, got %T: %v", err, err)
	}
	if pfErr.Code != procframe.CodeInternal {
		t.Fatalf("want CodeInternal, got %q", pfErr.Code)
	}
}

// --- --json input with streaming ---

func TestIntegration_JSONInput_Streaming(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := testv1.NewTickServiceCLIRunner(
		&tickHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"--json", `{"label":"json-stream","count":2}`,
		"tick", "watch",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Count(stdout.String(), "json-stream") != 2 {
		t.Fatalf("want 2 chunks, got:\n%s", stdout.String())
	}
}
