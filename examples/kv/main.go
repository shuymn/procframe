package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/shuymn/procframe"
	kvv1 "github.com/shuymn/procframe/examples/gen/kv/v1"
	"github.com/shuymn/procframe/transport/cli"
)

type handler struct {
	store map[string]string
}

func (h *handler) Get(
	_ context.Context,
	req *procframe.Request[kvv1.GetRequest],
) (*procframe.Response[kvv1.GetResponse], error) {
	v, ok := h.store[req.Msg.Key]
	if !ok {
		return nil, procframe.NewError(procframe.CodeNotFound, fmt.Sprintf("key %q not found", req.Msg.Key))
	}
	return &procframe.Response[kvv1.GetResponse]{
		Msg: &kvv1.GetResponse{
			Key:   req.Msg.Key,
			Value: v,
		},
	}, nil
}

func (h *handler) List(
	_ context.Context,
	req *procframe.Request[kvv1.ListRequest],
	stream procframe.ServerStream[kvv1.Entry],
) error {
	for _, k := range slices.Sorted(maps.Keys(h.store)) {
		if !strings.HasPrefix(k, req.Msg.Prefix) {
			continue
		}
		if err := stream.Send(&procframe.Response[kvv1.Entry]{
			Msg: &kvv1.Entry{
				Key:   k,
				Value: h.store[k],
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	h := &handler{
		store: map[string]string{
			"greeting":    "hello world",
			"greeting:ja": "こんにちは世界",
			"version":     "1.0.0",
		},
	}
	runner := kvv1.NewKVServiceCLIRunner(h)
	if err := runner.Run(context.Background(), os.Args[1:]); err != nil {
		if status, ok := procframe.StatusOf(err); ok {
			os.Exit(cli.ExitCode(status.Code))
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
