package cli_test

import (
	"bytes"
	"context"
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
