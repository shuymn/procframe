package cli_test

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/shuymn/procframe"
	"github.com/shuymn/procframe/transport/cli"
)

func TestRunner_SimpleCommand(t *testing.T) {
	t.Parallel()

	var called bool
	var gotArgs []string
	root := &cli.Node{
		Children: map[string]*cli.Node{
			"echo": {
				Segment: "echo",
				Run: func(_ context.Context, args []string, _ io.Writer) error {
					called = true
					gotArgs = args
					return nil
				},
			},
		},
	}
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&bytes.Buffer{}))
	err := r.Run(t.Context(), []string{"echo", "--message", "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("command was not called")
	}
	if len(gotArgs) != 2 || gotArgs[0] != "--message" || gotArgs[1] != "hello" {
		t.Fatalf("want [--message hello], got %v", gotArgs)
	}
}

func TestRunner_NestedGroups(t *testing.T) {
	t.Parallel()

	var called bool
	root := &cli.Node{
		Children: map[string]*cli.Node{
			"repo": {
				Segment: "repo",
				Children: map[string]*cli.Node{
					"pr": {
						Segment: "pr",
						Children: map[string]*cli.Node{
							"list": {
								Segment: "list",
								Run: func(_ context.Context, _ []string, _ io.Writer) error {
									called = true
									return nil
								},
							},
						},
					},
				},
			},
		},
	}
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&bytes.Buffer{}))
	err := r.Run(t.Context(), []string{"repo", "pr", "list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("nested command was not called")
	}
}

func TestRunner_GroupFlags(t *testing.T) {
	t.Parallel()

	var org string
	var gotArgs []string
	repoFS := flag.NewFlagSet("repo", flag.ContinueOnError)
	repoFS.StringVar(&org, "org", "", "organization")

	root := &cli.Node{
		Children: map[string]*cli.Node{
			"repo": {
				Segment: "repo",
				FlagSet: repoFS,
				Children: map[string]*cli.Node{
					"list": {
						Segment: "list",
						Run: func(_ context.Context, args []string, _ io.Writer) error {
							gotArgs = args
							return nil
						},
					},
				},
			},
		},
	}
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&bytes.Buffer{}))
	err := r.Run(t.Context(), []string{"repo", "--org", "myorg", "list", "--limit", "10"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if org != "myorg" {
		t.Fatalf("want org=myorg, got %q", org)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "--limit" || gotArgs[1] != "10" {
		t.Fatalf("want [--limit 10], got %v", gotArgs)
	}
}

func TestRunner_NoArgs(t *testing.T) {
	t.Parallel()

	root := &cli.Node{
		Summary: "root help",
		Children: map[string]*cli.Node{
			"cmd": {
				Segment: "cmd",
				Summary: "A command",
				Run:     func(context.Context, []string, io.Writer) error { return nil },
			},
		},
	}
	var stderr bytes.Buffer
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&stderr))
	err := r.Run(t.Context(), []string{})
	if err == nil {
		t.Fatal("expected error for no args")
	}
	if stderr.Len() == 0 {
		t.Fatal("expected help output on stderr")
	}
}

func TestRunner_HelpFlag(t *testing.T) {
	t.Parallel()

	root := &cli.Node{
		Children: map[string]*cli.Node{
			"cmd": {
				Segment: "cmd",
				Summary: "A command",
				Run:     func(context.Context, []string, io.Writer) error { return nil },
			},
		},
	}
	var stderr bytes.Buffer
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&stderr))
	err := r.Run(t.Context(), []string{"--help"})
	if err != nil {
		t.Fatalf("--help should not return error, got: %v", err)
	}
	if !strings.Contains(stderr.String(), "cmd") {
		t.Fatalf("want help to list commands, got:\n%s", stderr.String())
	}
}

func TestRunner_HelpFlagOnSubcommand(t *testing.T) {
	t.Parallel()

	root := &cli.Node{
		Children: map[string]*cli.Node{
			"repo": {
				Segment: "repo",
				Children: map[string]*cli.Node{
					"list": {
						Segment: "list",
						Summary: "List repos",
						Run:     func(context.Context, []string, io.Writer) error { return nil },
					},
				},
			},
		},
	}
	var stderr bytes.Buffer
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&stderr))
	err := r.Run(t.Context(), []string{"repo", "--help"})
	if err != nil {
		t.Fatalf("--help should not return error, got: %v", err)
	}
	if !strings.Contains(stderr.String(), "list") {
		t.Fatalf("want help to list subcommands, got:\n%s", stderr.String())
	}
}

func TestRunner_UnknownCommand(t *testing.T) {
	t.Parallel()

	root := &cli.Node{
		Children: map[string]*cli.Node{
			"cmd": {
				Segment: "cmd",
				Run:     func(context.Context, []string, io.Writer) error { return nil },
			},
		},
	}
	var stderr bytes.Buffer
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&stderr))
	err := r.Run(t.Context(), []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Fatalf("want error mentioning unknown command, got: %v", err)
	}
}

func TestRunner_NilRunDoesNotPanic(t *testing.T) {
	t.Parallel()

	root := &cli.Node{
		Children: map[string]*cli.Node{
			"broken": {
				Segment: "broken",
				// Run is nil — must not panic
			},
		},
	}
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&bytes.Buffer{}))
	err := r.Run(t.Context(), []string{"broken"})
	if err == nil {
		t.Fatal("expected error for nil Run")
	}
	if !strings.Contains(err.Error(), "not runnable") {
		t.Fatalf("want 'not runnable' error, got: %v", err)
	}
}

func TestRunner_HandlerError(t *testing.T) {
	t.Parallel()

	handlerErr := procframe.NewError(procframe.CodeNotFound, "not found")
	root := &cli.Node{
		Children: map[string]*cli.Node{
			"get": {
				Segment: "get",
				Run: func(context.Context, []string, io.Writer) error {
					return handlerErr
				},
			},
		},
	}
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&bytes.Buffer{}))
	err := r.Run(t.Context(), []string{"get"})
	if err == nil {
		t.Fatal("expected error from handler")
	}
	var pfErr procframe.Error
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected procframe.Error, got %T: %v", err, err)
	}
	if pfErr.Code() != procframe.CodeNotFound {
		t.Fatalf("want CodeNotFound, got %q", pfErr.Code())
	}
}

func TestRunner_PreParse_JSON(t *testing.T) {
	t.Parallel()

	var gotArgs []string
	root := &cli.Node{
		Children: map[string]*cli.Node{
			"cmd": {
				Segment: "cmd",
				Run: func(_ context.Context, args []string, _ io.Writer) error {
					gotArgs = args
					return nil
				},
			},
		},
	}
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&bytes.Buffer{}))
	err := r.Run(t.Context(), []string{"--json", `{"msg":"hi"}`, "cmd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotArgs) != 0 {
		t.Fatalf("want no remaining args, got %v", gotArgs)
	}
}

func TestRunner_PreParse_Output(t *testing.T) {
	t.Parallel()

	var gotFormat cli.OutputFormat
	root := &cli.Node{
		Children: map[string]*cli.Node{
			"cmd": {
				Segment: "cmd",
				Run: func(ctx context.Context, _ []string, _ io.Writer) error {
					gotFormat = cli.OutputFormatFromContext(ctx)
					return nil
				},
			},
		},
	}
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&bytes.Buffer{}))
	err := r.Run(t.Context(), []string{"--output", "json", "cmd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotFormat != cli.OutputJSON {
		t.Fatal("want OutputJSON in context")
	}
}

func TestRunner_PreParse_OutputInvalid(t *testing.T) {
	t.Parallel()

	root := &cli.Node{
		Children: map[string]*cli.Node{
			"cmd": {
				Segment: "cmd",
				Run:     func(context.Context, []string, io.Writer) error { return nil },
			},
		},
	}
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&bytes.Buffer{}))
	err := r.Run(t.Context(), []string{"--output", "xml", "cmd"})
	if err == nil {
		t.Fatal("expected error for invalid --output")
	}
	if !strings.Contains(err.Error(), "xml") {
		t.Fatalf("want error mentioning xml, got: %v", err)
	}
}

func TestRunner_PreParse_JSONMissing(t *testing.T) {
	t.Parallel()

	root := &cli.Node{
		Children: map[string]*cli.Node{
			"cmd": {
				Segment: "cmd",
				Run:     func(context.Context, []string, io.Writer) error { return nil },
			},
		},
	}
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&bytes.Buffer{}))
	err := r.Run(t.Context(), []string{"cmd", "--json"})
	if err == nil {
		t.Fatal("expected error for --json without value")
	}
}

func TestRunner_PreParse_JSONEqualsForm(t *testing.T) {
	t.Parallel()

	var gotArgs []string
	root := &cli.Node{
		Children: map[string]*cli.Node{
			"cmd": {
				Segment: "cmd",
				Run: func(_ context.Context, args []string, _ io.Writer) error {
					gotArgs = args
					return nil
				},
			},
		},
	}
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&bytes.Buffer{}))
	err := r.Run(t.Context(), []string{"--json={\"msg\":\"hi\"}", "cmd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotArgs) != 0 {
		t.Fatalf("want no remaining args, got %v", gotArgs)
	}
}

func TestRunner_PreParse_OutputEqualsForm(t *testing.T) {
	t.Parallel()

	var gotFormat cli.OutputFormat
	root := &cli.Node{
		Children: map[string]*cli.Node{
			"cmd": {
				Segment: "cmd",
				Run: func(ctx context.Context, _ []string, _ io.Writer) error {
					gotFormat = cli.OutputFormatFromContext(ctx)
					return nil
				},
			},
		},
	}
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&bytes.Buffer{}))
	err := r.Run(t.Context(), []string{"--output=json", "cmd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotFormat != cli.OutputJSON {
		t.Fatal("want OutputJSON")
	}
}

func TestRunner_PreParse_DuplicateJSON(t *testing.T) {
	t.Parallel()

	root := &cli.Node{
		Children: map[string]*cli.Node{
			"cmd": {
				Segment: "cmd",
				Run:     func(context.Context, []string, io.Writer) error { return nil },
			},
		},
	}
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&bytes.Buffer{}))
	err := r.Run(t.Context(), []string{"--json", "{}", "--json", "{}", "cmd"})
	if err == nil {
		t.Fatal("expected error for duplicate --json")
	}
	if !strings.Contains(err.Error(), "multiple times") {
		t.Fatalf("want 'multiple times' error, got: %v", err)
	}
}

func TestRunner_PreParse_DuplicateOutput(t *testing.T) {
	t.Parallel()

	root := &cli.Node{
		Children: map[string]*cli.Node{
			"cmd": {
				Segment: "cmd",
				Run:     func(context.Context, []string, io.Writer) error { return nil },
			},
		},
	}
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&bytes.Buffer{}))
	err := r.Run(t.Context(), []string{"--output", "json", "--output", "text", "cmd"})
	if err == nil {
		t.Fatal("expected error for duplicate --output")
	}
	if !strings.Contains(err.Error(), "multiple times") {
		t.Fatalf("want 'multiple times' error, got: %v", err)
	}
}

func TestRunner_StructuredErrorJSON(t *testing.T) {
	t.Parallel()

	root := &cli.Node{
		Children: map[string]*cli.Node{
			"cmd": {
				Segment: "cmd",
				Run: func(context.Context, []string, io.Writer) error {
					return procframe.NewError(procframe.CodeNotFound, "missing")
				},
			},
		},
	}
	var stderr bytes.Buffer
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&stderr))
	err := r.Run(t.Context(), []string{"--output", "json", "cmd"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stderr.String(), `"code":"not_found"`) {
		t.Fatalf("want structured error on stderr, got:\n%s", stderr.String())
	}
}

func TestRunner_StructuredErrorNotWrittenWithoutOutputJSON(t *testing.T) {
	t.Parallel()

	root := &cli.Node{
		Children: map[string]*cli.Node{
			"cmd": {
				Segment: "cmd",
				Run: func(context.Context, []string, io.Writer) error {
					return procframe.NewError(procframe.CodeNotFound, "missing")
				},
			},
		},
	}
	var stderr bytes.Buffer
	r := cli.NewRunner(root, cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&stderr))
	err := r.Run(t.Context(), []string{"cmd"})
	if err == nil {
		t.Fatal("expected error")
	}
	if stderr.Len() != 0 {
		t.Fatalf("want no structured error without --output json, got:\n%s", stderr.String())
	}
}

func TestRunner_HelpShowsProgramName(t *testing.T) {
	t.Parallel()

	root := &cli.Node{
		Children: map[string]*cli.Node{
			"greet": {
				Segment: "greet",
				Summary: "Greet someone",
				Run:     func(context.Context, []string, io.Writer) error { return nil },
			},
		},
	}
	var stderr bytes.Buffer
	r := cli.NewRunner(root, cli.WithName("myapp"), cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&stderr))
	err := r.Run(t.Context(), []string{"--help"})
	if err != nil {
		t.Fatalf("--help should not return error, got: %v", err)
	}
	if !strings.Contains(stderr.String(), "Usage: myapp") {
		t.Fatalf("want Usage containing program name, got:\n%s", stderr.String())
	}
}

func TestRunner_NoArgs_ErrorIncludesProgramName(t *testing.T) {
	t.Parallel()

	root := &cli.Node{
		Children: map[string]*cli.Node{
			"cmd": {
				Segment: "cmd",
				Summary: "A command",
				Run:     func(context.Context, []string, io.Writer) error { return nil },
			},
		},
	}
	r := cli.NewRunner(root, cli.WithName("myapp"), cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&bytes.Buffer{}))
	err := r.Run(t.Context(), []string{})
	if err == nil {
		t.Fatal("expected error for no args")
	}
	if !strings.Contains(err.Error(), "myapp") {
		t.Fatalf("want error mentioning program name, got: %v", err)
	}
}

func TestRunner_WithNameEmpty(t *testing.T) {
	t.Parallel()

	root := &cli.Node{
		Children: map[string]*cli.Node{
			"cmd": {
				Segment: "cmd",
				Summary: "A command",
				Run:     func(context.Context, []string, io.Writer) error { return nil },
			},
		},
	}
	r := cli.NewRunner(root, cli.WithName(""), cli.WithStdout(&bytes.Buffer{}), cli.WithStderr(&bytes.Buffer{}))
	err := r.Run(t.Context(), []string{})
	if err == nil {
		t.Fatal("expected error for no args")
	}
	if err.Error() != "no command specified" {
		t.Fatalf("want generic error without program name, got: %v", err)
	}
}
