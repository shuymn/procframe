package connect_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	connectrpc "connectrpc.com/connect"

	"github.com/shuymn/procframe"
	testv1 "github.com/shuymn/procframe/internal/gen/test/v1"
	connecttransport "github.com/shuymn/procframe/transport/connect"
)

func TestIntegration_ConnectInterceptor(t *testing.T) {
	t.Parallel()

	var called bool
	mux := http.NewServeMux()
	mux.Handle(connecttransport.NewUnaryHandler(
		"/test.v1.EchoService/Echo",
		func(
			context.Context,
			*procframe.Request[testv1.EchoRequest],
		) (*procframe.Response[testv1.EchoResponse], error) {
			called = true
			return &procframe.Response[testv1.EchoResponse]{
				Msg: &testv1.EchoResponse{Message: "handler"},
			}, nil
		},
		connecttransport.WithInterceptors(
			procframe.InterceptorFunc(func(_ procframe.HandlerFunc) procframe.HandlerFunc {
				return func(_ context.Context, conn procframe.Conn) error {
					if conn.Spec().Transport != procframe.TransportConnect {
						t.Fatalf("want Connect transport, got %q", conn.Spec().Transport)
					}
					return conn.Send(procframe.NewAnyResponse(&testv1.EchoResponse{Message: "intercepted"}))
				}
			}),
		),
	))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := connectrpc.NewClient[testv1.EchoRequest, testv1.EchoResponse](
		srv.Client(),
		srv.URL+"/test.v1.EchoService/Echo",
	)

	resp, err := client.CallUnary(t.Context(), connectrpc.NewRequest(&testv1.EchoRequest{
		Message: "hello",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("handler must not run")
	}
	if resp.Msg.Message != "intercepted" {
		t.Fatalf("want intercepted response, got %q", resp.Msg.Message)
	}
}
