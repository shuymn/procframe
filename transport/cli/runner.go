package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/shuymn/procframe"
)

// Runner holds a CLI command tree and dispatches execution.
type Runner struct {
	root        *Node
	name        string
	stdout      io.Writer
	stderr      io.Writer
	errorMapper procframe.ErrorMapper
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

// WithName sets the program name shown in usage and error messages.
func WithName(name string) Option {
	return func(r *Runner) { r.name = name }
}

// WithErrorMapper sets the boundary mapper used to classify errors.
func WithErrorMapper(mapper procframe.ErrorMapper) Option {
	return func(r *Runner) { r.errorMapper = mapper }
}

// NewRunner constructs a [Runner] from a root group node.
func NewRunner(root *Node, opts ...Option) *Runner {
	r := &Runner{
		root:   root,
		name:   filepath.Base(os.Args[0]),
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
	remaining, gf, err := preParseGlobalFlags(args)
	if err != nil {
		return err
	}
	if gf.jsonPayload != "" {
		ctx = WithJSONPayload(ctx, gf.jsonPayload)
	}
	if gf.outputFmt != "" {
		ctx = WithOutputFormat(ctx, gf.outputFmt)
	}

	path := []string{}
	if r.name != "" {
		path = []string{r.name}
	}
	runErr := r.traverse(ctx, r.root, remaining, path)
	if runErr != nil {
		status, ok, mappedErr := r.mapError(runErr)
		if ok && OutputFormatFromContext(ctx) == OutputJSON {
			//nolint:errcheck // best-effort structured error output
			FormatErrorJSON(r.stderr, status)
		}
		return mappedErr
	}
	return nil
}

func (r *Runner) mapError(err error) (procframe.Status, bool, error) {
	if status, ok := procframe.StatusOf(err); ok {
		return status, true, err
	}
	if r.errorMapper == nil {
		return procframe.Status{}, false, err
	}
	status, ok := r.errorMapper(err)
	if !ok {
		return procframe.Status{}, false, err
	}
	mapped := procframe.WrapError(status.Code, status.Message, err)
	if status.Retryable {
		mapped = mapped.WithRetryable()
	}
	return status, true, mapped
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

// globalFlags holds the extracted global flags from pre-parsing.
type globalFlags struct {
	jsonPayload string
	outputFmt   OutputFormat
}

// preParseGlobalFlags extracts --json and --output from args before
// the command tree sees them. Returns the remaining args plus the
// extracted global flags.
func preParseGlobalFlags(args []string) ([]string, globalFlags, error) {
	remaining := make([]string, 0, len(args))
	var gf globalFlags
	var jsonSeen, outputSeen bool

	for i := 0; i < len(args); i++ {
		key, val, consumed := splitGlobalFlag(args, i)
		if key == "" {
			remaining = append(remaining, args[i])
			continue
		}
		if val == "" {
			return nil, globalFlags{}, fmt.Errorf("%s requires a value", key)
		}
		var err error
		switch key {
		case "--json":
			err = setOnce(&gf.jsonPayload, val, key, &jsonSeen)
		case "--output":
			err = setOutputFormat(&gf.outputFmt, val, &outputSeen)
		}
		if err != nil {
			return nil, globalFlags{}, err
		}
		i += consumed
	}
	return remaining, gf, nil
}

func setOnce(dst *string, val, name string, seen *bool) error {
	if *seen {
		return fmt.Errorf("%s specified multiple times", name)
	}
	*seen = true
	*dst = val
	return nil
}

func setOutputFormat(dst *OutputFormat, val string, seen *bool) error {
	if *seen {
		return fmt.Errorf("--output specified multiple times")
	}
	*seen = true
	switch val {
	case "text":
		*dst = OutputText
	case "json":
		*dst = OutputJSON
	default:
		return fmt.Errorf("--output must be \"text\" or \"json\", got %q", val)
	}
	return nil
}

// splitGlobalFlag checks if args[i] is a global flag (--json or --output)
// and extracts its value. Supports both "--flag value" and "--flag=value" forms.
// Returns (key, value, extraArgsConsumed).
// If args[i] is not a global flag, returns ("", "", 0).
func splitGlobalFlag(args []string, i int) (string, string, int) {
	arg := args[i]
	for _, prefix := range []string{"--json", "--output"} {
		if arg == prefix {
			if i+1 < len(args) {
				return prefix, args[i+1], 1
			}
			return prefix, "", 0
		}
		if strings.HasPrefix(arg, prefix+"=") {
			return prefix, arg[len(prefix)+1:], 0
		}
	}
	return "", "", 0
}
