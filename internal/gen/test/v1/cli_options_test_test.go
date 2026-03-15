package testv1

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/shuymn/procframe"
	"github.com/shuymn/procframe/transport/cli"
)

// stubCliOptionsHandler implements CliOptionsTestServiceHandler with no-op responses.
type stubCliOptionsHandler struct{}

func (stubCliOptionsHandler) DefaultEnabled(
	_ context.Context, _ *procframe.Request[PingRequest],
) (*procframe.Response[PingResponse], error) {
	return &procframe.Response[PingResponse]{Msg: &PingResponse{Result: "ok"}}, nil
}

func (stubCliOptionsHandler) ExplicitEnabled(
	_ context.Context, _ *procframe.Request[PingRequest],
) (*procframe.Response[PingResponse], error) {
	return &procframe.Response[PingResponse]{Msg: &PingResponse{Result: "ok"}}, nil
}

func (stubCliOptionsHandler) ExplicitDisabled(
	_ context.Context, _ *procframe.Request[PingRequest],
) (*procframe.Response[PingResponse], error) {
	return &procframe.Response[PingResponse]{Msg: &PingResponse{Result: "ok"}}, nil
}

func (stubCliOptionsHandler) WsEnabled(
	_ context.Context, _ *procframe.Request[PingRequest],
) (*procframe.Response[PingResponse], error) {
	return &procframe.Response[PingResponse]{Msg: &PingResponse{Result: "ok"}}, nil
}

func TestCliOptionsTestService_CLIRouting(t *testing.T) {
	t.Parallel()

	runner := NewCliOptionsTestServiceCLIRunner(
		stubCliOptionsHandler{},
		cli.WithStdout(io.Discard),
		cli.WithStderr(io.Discard),
	)
	ctx := t.Context()

	tests := []struct {
		name    string
		args    []string
		wantErr string // empty means no error expected
	}{
		{
			name: "default enabled is reachable",
			args: []string{"cliopts", "default-enabled"},
		},
		{
			name: "explicit enabled is reachable",
			args: []string{"cliopts", "explicit-enabled"},
		},
		{
			name:    "explicit disabled is excluded",
			args:    []string{"cliopts", "explicit-disabled"},
			wantErr: `unknown command "explicit-disabled"`,
		},
		{
			name: "ws enabled does not affect CLI routing",
			args: []string{"cliopts", "ws-enabled"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := runner.Run(ctx, tt.args)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("want no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}
