package cli_test

import (
	"bytes"
	"context"
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/shuymn/procframe/transport/cli"
)

func TestWriteGroupHelp(t *testing.T) {
	t.Parallel()

	t.Run("lists visible children", func(t *testing.T) {
		t.Parallel()
		group := &cli.Node{
			Summary: "Repository operations",
			Children: map[string]*cli.Node{
				"list": {
					Segment: "list",
					Summary: "List items",
					Run:     func(context.Context, []string, io.Writer) error { return nil },
				},
				"get": {
					Segment: "get",
					Summary: "Get an item",
					Run:     func(context.Context, []string, io.Writer) error { return nil },
				},
			},
		}
		var buf bytes.Buffer
		cli.WriteGroupHelp(&buf, group, []string{"app", "repo"})
		out := buf.String()

		if !strings.Contains(out, "app repo") {
			t.Fatalf("want path in output, got:\n%s", out)
		}
		if !strings.Contains(out, "list") || !strings.Contains(out, "List items") {
			t.Fatalf("want list command in output, got:\n%s", out)
		}
		if !strings.Contains(out, "get") || !strings.Contains(out, "Get an item") {
			t.Fatalf("want get command in output, got:\n%s", out)
		}
	})

	t.Run("excludes hidden children", func(t *testing.T) {
		t.Parallel()
		group := &cli.Node{
			Children: map[string]*cli.Node{
				"visible": {
					Segment: "visible",
					Summary: "Visible command",
					Run:     func(context.Context, []string, io.Writer) error { return nil },
				},
				"secret": {
					Segment: "secret",
					Summary: "Secret command",
					Hidden:  true,
					Run:     func(context.Context, []string, io.Writer) error { return nil },
				},
			},
		}
		var buf bytes.Buffer
		cli.WriteGroupHelp(&buf, group, []string{"app"})
		out := buf.String()

		if !strings.Contains(out, "visible") {
			t.Fatalf("want visible command in output, got:\n%s", out)
		}
		if strings.Contains(out, "secret") {
			t.Fatalf("want hidden command excluded, got:\n%s", out)
		}
	})

	t.Run("shows group flags", func(t *testing.T) {
		t.Parallel()
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		fs.String("org", "", "organization name")
		group := &cli.Node{
			FlagSet: fs,
			Children: map[string]*cli.Node{
				"list": {
					Segment: "list",
					Run:     func(context.Context, []string, io.Writer) error { return nil },
				},
			},
		}
		var buf bytes.Buffer
		cli.WriteGroupHelp(&buf, group, []string{"app", "repo"})
		out := buf.String()

		if !strings.Contains(out, "--org") {
			t.Fatalf("want group flag in output, got:\n%s", out)
		}
	})
}

func TestWriteCommandHelp(t *testing.T) {
	t.Parallel()

	t.Run("shows command with flags", func(t *testing.T) {
		t.Parallel()
		fs := flag.NewFlagSet("list", flag.ContinueOnError)
		fs.Int("limit", 0, "max items to return")
		fs.String("state", "", "filter by state")

		cmd := &cli.Node{
			Segment: "list",
			Summary: "List pull requests",
			Run:     func(context.Context, []string, io.Writer) error { return nil },
		}
		var buf bytes.Buffer
		cli.WriteCommandHelp(&buf, cmd, []string{"app", "repo", "pr", "list"}, fs)
		out := buf.String()

		if !strings.Contains(out, "app repo pr list") {
			t.Fatalf("want full path in output, got:\n%s", out)
		}
		if !strings.Contains(out, "List pull requests") {
			t.Fatalf("want summary in output, got:\n%s", out)
		}
		if !strings.Contains(out, "--limit") {
			t.Fatalf("want --limit flag in output, got:\n%s", out)
		}
		if !strings.Contains(out, "--state") {
			t.Fatalf("want --state flag in output, got:\n%s", out)
		}
	})
}
