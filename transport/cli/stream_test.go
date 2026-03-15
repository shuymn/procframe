package cli_test

import (
	"errors"
	"testing"

	"github.com/shuymn/procframe"
	"github.com/shuymn/procframe/transport/cli"
)

func TestStreamWriter_ContextReturned(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	sw := cli.NewStreamWriter[string](ctx, func(_ *procframe.Response[string]) error {
		return nil
	})
	if sw.Context() != ctx {
		t.Fatal("want same context")
	}
}

func TestStreamWriter_SendCallsWrite(t *testing.T) {
	t.Parallel()

	var called int
	sw := cli.NewStreamWriter[string](t.Context(), func(resp *procframe.Response[string]) error {
		called++
		if resp.Msg == nil || *resp.Msg != "hello" {
			t.Fatalf("unexpected msg: %v", resp.Msg)
		}
		return nil
	})

	msg := "hello"
	if err := sw.Send(&procframe.Response[string]{Msg: &msg}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != 1 {
		t.Fatalf("want 1 call, got %d", called)
	}
}

func TestStreamWriter_SendPropagatesError(t *testing.T) {
	t.Parallel()

	want := &procframe.Error{Code: procframe.CodeInternal, Message: "boom"}
	sw := cli.NewStreamWriter[string](t.Context(), func(_ *procframe.Response[string]) error {
		return want
	})

	msg := "test"
	err := sw.Send(&procframe.Response[string]{Msg: &msg})
	if !errors.Is(err, want) {
		t.Fatalf("want error propagated, got %v", err)
	}
}
