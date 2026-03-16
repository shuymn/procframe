package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/shuymn/procframe"
	echov1 "github.com/shuymn/procframe/examples/gen/echo/v1"
	"github.com/shuymn/procframe/transport/cli"
)

type handler struct{}

func (h *handler) Echo(
	_ context.Context,
	req *procframe.Request[echov1.EchoRequest],
) (*procframe.Response[echov1.EchoResponse], error) {
	return &procframe.Response[echov1.EchoResponse]{
		Msg: &echov1.EchoResponse{
			Message: req.Msg.Message,
		},
	}, nil
}

func main() {
	runner := echov1.NewEchoServiceCLIRunner(&handler{})
	if err := runner.Run(context.Background(), os.Args[1:]); err != nil {
		var pfErr procframe.Error
		if errors.As(err, &pfErr) {
			os.Exit(cli.ExitCode(pfErr.Code()))
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
