package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// Runner holds a CLI command tree and dispatches execution.
type Runner struct {
	root   *Node
	stdout io.Writer
	stderr io.Writer
}

// Option configures a [Runner].
type Option func(*Runner)

// WithStdout sets the writer for standard output.
func WithStdout(w io.Writer) Option {
	return func(r *Runner) { r.stdout = w }
}

// WithStderr sets the writer for standard error.
func WithStderr(w io.Writer) Option {
	return func(r *Runner) { r.stderr = w }
}

// NewRunner constructs a [Runner] from a root group node.
func NewRunner(root *Node, opts ...Option) *Runner {
	r := &Runner{
		root:   root,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Stdout returns the writer used for standard output.
func (r *Runner) Stdout() io.Writer { return r.stdout }

// Stderr returns the writer used for standard error.
func (r *Runner) Stderr() io.Writer { return r.stderr }

// Run parses args, traverses the command tree, and executes the
// matched leaf command. Returns nil on success or help display.
func (r *Runner) Run(ctx context.Context, args []string) error {
	return r.traverse(ctx, r.root, args, []string{})
}

// errHelp is a sentinel indicating that help was displayed.
var errHelp = errors.New("help displayed")

func (r *Runner) traverse(ctx context.Context, node *Node, args, path []string) error {
	args, err := r.parseNodeFlags(node, args, path)
	if errors.Is(err, errHelp) {
		return nil
	}
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return r.handleNoArgs(ctx, node, args, path)
	}

	return r.dispatch(ctx, node, args, path)
}

// parseNodeFlags parses the flag set on the node if present.
// Returns errHelp if --help was handled.
func (r *Runner) parseNodeFlags(node *Node, args, path []string) ([]string, error) {
	if node.FlagSet == nil {
		return args, nil
	}
	node.FlagSet.SetOutput(io.Discard)
	if err := node.FlagSet.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if node.IsGroup() {
				WriteGroupHelp(r.stderr, node, path)
			}
			return nil, errHelp
		}
		return nil, err
	}
	return node.FlagSet.Args(), nil
}

// handleNoArgs handles the case when no arguments remain after flag parsing.
func (r *Runner) handleNoArgs(ctx context.Context, node *Node, args, path []string) error {
	if node.IsGroup() {
		WriteGroupHelp(r.stderr, node, path)
		if len(path) == 0 {
			return fmt.Errorf("no command specified")
		}
		return fmt.Errorf("no command specified for %q", path[len(path)-1])
	}
	if node.Run != nil {
		return node.Run(ctx, args, r.stdout)
	}
	return fmt.Errorf("no command specified")
}

// dispatch resolves the next command name and dispatches to the child node.
func (r *Runner) dispatch(ctx context.Context, node *Node, args, path []string) error {
	name := args[0]

	if name == "--help" || name == "-h" {
		if node.IsGroup() {
			WriteGroupHelp(r.stderr, node, path)
		}
		return nil
	}

	if !node.IsGroup() {
		return fmt.Errorf("unknown argument %q", name)
	}

	child, ok := node.Children[name]
	if !ok {
		return fmt.Errorf("unknown command %q", name)
	}

	childPath := append([]string{}, path...)
	childPath = append(childPath, name)
	if child.IsGroup() {
		return r.traverse(ctx, child, args[1:], childPath)
	}

	remaining := args[1:]
	if len(remaining) > 0 && (remaining[0] == "--help" || remaining[0] == "-h") {
		WriteCommandHelp(r.stderr, child, childPath, nil)
		return nil
	}

	if child.Run == nil {
		return fmt.Errorf("command %q is not runnable", name)
	}
	return child.Run(ctx, remaining, r.stdout)
}
