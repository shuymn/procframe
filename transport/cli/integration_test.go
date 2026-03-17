package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/shuymn/procframe"
	testv1 "github.com/shuymn/procframe/internal/gen/test/v1"
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
	return nil, procframe.NewError(procframe.CodeNotFound, "resource not found")
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

	status, ok := procframe.StatusOf(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if status.Code != procframe.CodeNotFound {
		t.Fatalf("want CodeNotFound, got %q", status.Code)
	}

	exitCode := cli.ExitCode(status.Code)
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
	status, ok := procframe.StatusOf(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if status.Code != procframe.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %q", status.Code)
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
	if !strings.Contains(out, `"echo run"`) {
		t.Fatalf("want command path in schema, got:\n%s", out)
	}
	if !strings.Contains(out, `"Echo a message"`) {
		t.Fatalf("want summary in schema, got:\n%s", out)
	}
}

func TestIntegration_SchemaSpecificCommand(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := testv1.NewEchoServiceCLIRunner(
		&echoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	// Lookup by command path
	err := runner.Run(t.Context(), []string{"schema", "echo", "run"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var info cli.CommandInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout.String())
	}
	if info.Command != "echo run" {
		t.Fatalf("want command=%q, got %q", "echo run", info.Command)
	}
	if info.Procedure != "/test.v1.EchoService/Echo" {
		t.Fatalf("want procedure, got %q", info.Procedure)
	}
	if len(info.Flags) != 3 {
		t.Fatalf("want 3 flags, got %d", len(info.Flags))
	}
}

func TestIntegration_SchemaFallbackByProcedure(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := testv1.NewEchoServiceCLIRunner(
		&echoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	// Fallback: lookup by procedure name
	err := runner.Run(t.Context(), []string{"schema", "/test.v1.EchoService/Echo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var info cli.CommandInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout.String())
	}
	if info.Command != "echo run" {
		t.Fatalf("want command=%q, got %q", "echo run", info.Command)
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

	err := runner.Run(t.Context(), []string{"schema", "tick", "watch"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var info cli.CommandInfo
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

	status, ok := procframe.StatusOf(err)
	if !ok {
		t.Fatalf("expected status error, got %T", err)
	}

	code := cli.ExitCode(status.Code)
	if code != 3 { // CodeNotFound → 3
		t.Fatalf("want exit code 3, got %d", code)
	}
}

func TestIntegration_StreamingExitCode(t *testing.T) {
	t.Parallel()

	runner := testv1.NewTickServiceCLIRunner(
		&tickHandler{watchErr: procframe.NewError(procframe.CodeUnavailable, "service down")},
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"tick", "watch", "--label", "x", "--count", "1"})
	if err == nil {
		t.Fatal("expected error")
	}

	status, ok := procframe.StatusOf(err)
	if !ok {
		t.Fatalf("expected status error, got %T", err)
	}
	if cli.ExitCode(status.Code) != 8 { // CodeUnavailable → 8
		t.Fatalf("want exit code 8, got %d", cli.ExitCode(status.Code))
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
		cli.WithErrorMapper(procframe.StatusOf),
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
	status, ok := procframe.StatusOf(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if status.Code != procframe.CodeInternal {
		t.Fatalf("want CodeInternal, got %q", status.Code)
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

// --- Help metadata propagation ---

func TestIntegration_HelpShowsFieldDescriptions(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	runner := testv1.NewEchoServiceCLIRunner(
		&echoHandler{},
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&stderr),
	)

	err := runner.Run(t.Context(), []string{"echo", "run", "--help"})
	if err != nil {
		t.Fatalf("--help should not error, got: %v", err)
	}

	out := stderr.String()
	for _, want := range []string{
		"The message to echo back",
		"Number of times to repeat",
		"Convert to uppercase",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("want %q in help output, got:\n%s", want, out)
		}
	}
}

func TestIntegration_PRServiceBindIntoLabels(t *testing.T) {
	t.Parallel()

	h := &prHandler{}
	runner := testv1.NewPRServiceCLIRunner(
		h,
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"repo", "pr", "--state", "open", "--labels", "bug", "--labels", "urgent",
		"list", "--limit", "5",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h.lastReq == nil || h.lastReq.Pr == nil {
		t.Fatal("want Pr field injected, got nil")
	}
	if len(h.lastReq.Pr.Labels) != 2 || h.lastReq.Pr.Labels[0] != "bug" || h.lastReq.Pr.Labels[1] != "urgent" {
		t.Fatalf("want labels=[bug urgent], got %v", h.lastReq.Pr.Labels)
	}
}

func TestIntegration_PRServiceBindIntoPrimaryLabel(t *testing.T) {
	t.Parallel()

	h := &prHandler{}
	runner := testv1.NewPRServiceCLIRunner(
		h,
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"repo", "pr", "--primary-label", `{"name":"critical"}`,
		"list",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h.lastReq == nil || h.lastReq.Pr == nil {
		t.Fatal("want Pr field injected, got nil")
	}
	if h.lastReq.Pr.PrimaryLabel == nil {
		t.Fatal("want PrimaryLabel set, got nil")
	}
	if h.lastReq.Pr.PrimaryLabel.Name != "critical" {
		t.Fatalf("want name=critical, got %q", h.lastReq.Pr.PrimaryLabel.Name)
	}
}

func TestIntegration_MessageFieldJSONFlag(t *testing.T) {
	t.Parallel()

	h := &prHandler{}
	var stdout bytes.Buffer
	runner := testv1.NewPRServiceCLIRunner(
		h,
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"repo", "pr", "list", "--repo", `{"org":"myorg"}`, "--limit", "3",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h.lastReq == nil {
		t.Fatal("handler was not called")
	}
	if h.lastReq.Repo == nil {
		t.Fatal("want Repo set via JSON flag, got nil")
	}
	if h.lastReq.Repo.Org != "myorg" {
		t.Fatalf("want org=myorg, got %q", h.lastReq.Repo.Org)
	}
	if h.lastReq.Limit != 3 {
		t.Fatalf("want limit=3, got %d", h.lastReq.Limit)
	}
}

func TestIntegration_HelpShowsEnumValues(t *testing.T) {
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
	if !strings.Contains(out, "values: open, closed") {
		t.Fatalf("want enum values in help output, got:\n%s", out)
	}
	if !strings.Contains(out, "Filter by PR state") {
		t.Fatalf("want field description in help output, got:\n%s", out)
	}
}

func TestIntegration_SchemaContainsDescription(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runner := testv1.NewEchoServiceCLIRunner(
		&echoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"schema", "echo", "run"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var info cli.CommandInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout.String())
	}

	wantDescs := map[string]string{
		"message":   "The message to echo back",
		"count":     "Number of times to repeat",
		"uppercase": "Convert to uppercase",
	}
	for _, f := range info.Flags {
		want, ok := wantDescs[f.Name]
		if !ok {
			continue
		}
		if f.Description != want {
			t.Errorf("flag %q: want description %q, got %q", f.Name, want, f.Description)
		}
	}
}

func BenchmarkSchemaList(b *testing.B) {
	var stdout, stderr bytes.Buffer
	runner := testv1.NewEchoServiceCLIRunner(
		&echoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&stderr),
	)

	args := []string{"schema"}
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		stdout.Reset()
		stderr.Reset()
		if err := runner.Run(b.Context(), args); err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}

func BenchmarkSchemaLookupByProcedure(b *testing.B) {
	var stdout, stderr bytes.Buffer
	runner := testv1.NewEchoServiceCLIRunner(
		&echoHandler{},
		cli.WithStdout(&stdout),
		cli.WithStderr(&stderr),
	)

	args := []string{"schema", "/test.v1.EchoService/Echo"}
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		stdout.Reset()
		stderr.Reset()
		if err := runner.Run(b.Context(), args); err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
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

func TestIntegration_FourShape_ClientStream(t *testing.T) {
	t.Parallel()

	stdin := strings.NewReader(
		"{\"item\":\"a\"}\n{\"item\":\"b\"}\n{\"item\":\"c\"}\n",
	)

	var stdout bytes.Buffer
	runner := testv1.NewFourShapeServiceCLIRunner(
		&fourShapeHandler{},
		cli.WithStdin(stdin),
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"--output", "json",
		"four", "collect",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := strings.TrimSpace(stdout.String())
	// With --output json, response should be compact single-line JSON
	if strings.Count(out, "\n") > 0 {
		t.Fatalf("want compact single-line JSON, got:\n%s", out)
	}

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, out)
	}
	if !strings.Contains(out, "a,b,c") {
		t.Fatalf("want items=a,b,c in output, got:\n%s", out)
	}
}

func TestIntegration_FourShape_Bidi(t *testing.T) {
	t.Parallel()

	stdin := strings.NewReader(
		"{\"text\":\"hello\"}\n{\"text\":\"world\"}\n",
	)

	var stdout bytes.Buffer
	runner := testv1.NewFourShapeServiceCLIRunner(
		&fourShapeHandler{},
		cli.WithStdin(stdin),
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"--output", "json",
		"four", "chat",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "echo:hello") {
		t.Fatalf("want echo:hello in output, got:\n%s", out)
	}
	if !strings.Contains(out, "echo:world") {
		t.Fatalf("want echo:world in output, got:\n%s", out)
	}

	// With --output json, each response is a compact NDJSON line
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 NDJSON lines, got %d:\n%s", len(lines), out)
	}
	for _, line := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &m); err != nil {
			t.Fatalf("invalid JSON line: %v\nline: %s", err, line)
		}
	}
}

func TestIntegration_FourShape_ClientStreamRejectsFlags(t *testing.T) {
	t.Parallel()

	runner := testv1.NewFourShapeServiceCLIRunner(
		&fourShapeHandler{},
		cli.WithStdin(strings.NewReader("")),
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"four", "collect", "--item", "x"})
	if err == nil {
		t.Fatal("expected error for client-stream with flags")
	}
	status, ok := procframe.StatusOf(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if status.Code != procframe.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %q", status.Code)
	}
}

func TestIntegration_FourShape_BidiRejectsFlags(t *testing.T) {
	t.Parallel()

	runner := testv1.NewFourShapeServiceCLIRunner(
		&fourShapeHandler{},
		cli.WithStdin(strings.NewReader("")),
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{"four", "chat", "--text", "hi"})
	if err == nil {
		t.Fatal("expected error for bidi with flags")
	}
	status, ok := procframe.StatusOf(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if status.Code != procframe.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %q", status.Code)
	}
}

func TestIntegration_FourShape_ClientStreamRejectsJSON(t *testing.T) {
	t.Parallel()

	runner := testv1.NewFourShapeServiceCLIRunner(
		&fourShapeHandler{},
		cli.WithStdin(strings.NewReader("")),
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"--json", `{"item":"x"}`,
		"four", "collect",
	})
	if err == nil {
		t.Fatal("expected error for client-stream with --json")
	}
	status, ok := procframe.StatusOf(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if status.Code != procframe.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %q", status.Code)
	}
}

func TestIntegration_FourShape_BidiRejectsJSON(t *testing.T) {
	t.Parallel()

	runner := testv1.NewFourShapeServiceCLIRunner(
		&fourShapeHandler{},
		cli.WithStdin(strings.NewReader("")),
		cli.WithStdout(&bytes.Buffer{}),
		cli.WithStderr(&bytes.Buffer{}),
	)

	err := runner.Run(t.Context(), []string{
		"--json", `{"text":"hi"}`,
		"four", "chat",
	})
	if err == nil {
		t.Fatal("expected error for bidi with --json")
	}
	status, ok := procframe.StatusOf(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if status.Code != procframe.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %q", status.Code)
	}
}

// errorExposingHandler returns errors that simulate internal detail leakage.
type errorExposingHandler struct{}

func (*errorExposingHandler) Echo(
	_ context.Context,
	_ *procframe.Request[testv1.EchoRequest],
) (*procframe.Response[testv1.EchoResponse], error) {
	return nil, errors.New("connection to database at 10.0.0.5:5432 refused")
}

// Ensure the interface is satisfied at compile time.
var _ testv1.EchoServiceHandler = (*errorExposingHandler)(nil)

// TestIntegration_StructuredErrorExposure verifies that structured error
// output (--output json) does not leak Go runtime internals.
func TestIntegration_StructuredErrorExposure(t *testing.T) {
	t.Parallel()

	h := &errorExposingHandler{}

	t.Run("handler_error_no_internals", func(t *testing.T) {
		t.Parallel()
		runner := testv1.NewEchoServiceCLIRunner(
			h,
			cli.WithStdout(io.Discard),
			cli.WithStderr(&bytes.Buffer{}),
		)
		err := runner.Run(t.Context(), []string{
			"--output", "json",
			"echo", "run", "--message", "trigger-error",
		})
		if err == nil {
			t.Fatal("expected error from handler")
		}
		checkNoInternalExposure(t, err.Error())
	})

	t.Run("unknown_command_error_no_internals", func(t *testing.T) {
		t.Parallel()
		runner := testv1.NewEchoServiceCLIRunner(
			h,
			cli.WithStdout(io.Discard),
			cli.WithStderr(&bytes.Buffer{}),
		)
		err := runner.Run(t.Context(), []string{
			"--output", "json",
			"nonexistent-command",
		})
		if err == nil {
			t.Fatal("expected error for unknown command")
		}
		checkNoInternalExposure(t, err.Error())
	})
}
