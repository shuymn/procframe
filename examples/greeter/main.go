package main

import (
	"context"
	"fmt"
	"os"

	"github.com/shuymn/procframe"
	"github.com/shuymn/procframe/config"
	greeterv1 "github.com/shuymn/procframe/examples/gen/greeter/v1"
	"github.com/shuymn/procframe/transport/cli"
)

type handler struct {
	cfg *greeterv1.GreeterConfig
}

func (h *handler) Greet(
	_ context.Context,
	req *procframe.Request[greeterv1.GreetRequest],
) (*procframe.Response[greeterv1.GreetResponse], error) {
	greeting := fmt.Sprintf("%s %s%s", h.cfg.Prefix, req.Msg.Name, h.cfg.Suffix)
	return &procframe.Response[greeterv1.GreetResponse]{
		Msg: &greeterv1.GreetResponse{
			Greeting: greeting,
		},
	}, nil
}

func main() {
	cfg, rest, err := config.Load[greeterv1.GreeterConfig](os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	runner := greeterv1.NewGreeterServiceCLIRunner(&handler{cfg: cfg})
	if err := runner.Run(context.Background(), rest); err != nil {
		if status, ok := procframe.StatusOf(err); ok {
			os.Exit(cli.ExitCode(status.Code))
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
