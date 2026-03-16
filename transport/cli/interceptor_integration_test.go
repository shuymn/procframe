package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/shuymn/procframe"
	testv1 "github.com/shuymn/procframe/internal/gen/test/v1"
	"github.com/shuymn/procframe/transport/cli"
)

type interceptingCLIHandler struct {
	called bool
}

func (h *interceptingCLIHandler) Echo(
	_ context.Context,
	_ *procframe.Request[testv1.EchoRequest],
) (*procframe.Response[testv1.EchoResponse], error) {
	h.called = true
	return &procframe.Response[testv1.EchoResponse]{
		Msg: &testv1.EchoResponse{Message: "handler"},
	}, nil
}

func TestIntegration_CLIInterceptor(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	h := &interceptingCLIHandler{}
	runner := testv1.NewEchoServiceCLIRunner(
		h,
		cli.WithStdout(&stdout),
		cli.WithStderr(&bytes.Buffer{}),
		cli.WithInterceptors(
			procframe.InterceptorFunc(func(_ procframe.HandlerFunc) procframe.HandlerFunc {
				return func(_ context.Context, conn procframe.Conn) error {
					if conn.Spec().Transport != procframe.TransportCLI {
						t.Fatalf("want CLI transport, got %q", conn.Spec().Transport)
					}
					return conn.Send(procframe.NewAnyResponse(&testv1.EchoResponse{Message: "intercepted", Count: 42}))
				}
			}),
		),
	)

	if err := runner.Run(t.Context(), []string{"echo", "run", "--message", "hello"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.called {
		t.Fatal("handler must not run")
	}
	out := stdout.String()
	if !strings.Contains(out, `"intercepted"`) {
		t.Fatalf("want intercepted output, got:\n%s", out)
	}
}
