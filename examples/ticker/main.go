package main

import (
	"context"
	"fmt"
	"os"

	"github.com/shuymn/procframe"
	tickerv1 "github.com/shuymn/procframe/examples/gen/ticker/v1"
	"github.com/shuymn/procframe/transport/cli"
)

type handler struct{}

func (h *handler) Tick(
	_ context.Context,
	req *procframe.Request[tickerv1.TickRequest],
	stream procframe.ServerStream[tickerv1.TickResponse],
) error {
	for i := range req.Msg.Count {
		if err := stream.Send(&procframe.Response[tickerv1.TickResponse]{
			Msg: &tickerv1.TickResponse{
				Message: fmt.Sprintf("%s-%d", req.Msg.Prefix, i+1),
				Seq:     i + 1,
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	runner := tickerv1.NewTickerServiceCLIRunner(&handler{})
	if err := runner.Run(context.Background(), os.Args[1:]); err != nil {
		if status, ok := procframe.StatusOf(err); ok {
			os.Exit(cli.ExitCode(status.Code))
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
