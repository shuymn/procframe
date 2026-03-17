package cli

import (
	"context"
	"flag"
	"io"
)

// Node represents a node in the CLI command tree. A node is either a
// group (Children != nil) or a leaf command (Run != nil).
type Node struct {
	// Segment is the path component for this node (e.g. "list").
	Segment string

	// Summary is a short description shown in help text.
	Summary string

	// Hidden excludes this node from help listings.
	Hidden bool

	// FlagSet holds group-level flags parsed before descending into children.
	// May be nil for nodes without group flags.
	FlagSet *flag.FlagSet

	// HelpFlags returns a FlagSet describing the leaf command's flags
	// for display in help output. May be nil. Not used for parsing.
	HelpFlags func() *flag.FlagSet

	// Children maps subcommand names to child nodes.
	// Non-nil for group nodes.
	Children map[string]*Node

	// Run is the leaf execution function. It receives remaining args
	// after flag parsing and the stdout writer for output. Non-nil
	// for leaf nodes.
	Run func(ctx context.Context, args []string, stdout io.Writer) error
}

// isGroup returns true if the node is a group (has children).
func (n *Node) isGroup() bool {
	return n.Children != nil
}
