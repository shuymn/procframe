package cli

import (
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
)

// writeGroupHelp writes help text for a group node to w.
// path is the command path leading to this group (e.g. ["app", "repo"]).
func writeGroupHelp(w io.Writer, g *Node, path []string) {
	pathStr := strings.Join(path, " ")
	fmt.Fprintf(w, "Usage: %s <command>\n", pathStr)

	if g.Summary != "" {
		fmt.Fprintf(w, "\n%s\n", g.Summary)
	}

	visible := visibleChildren(g)
	if len(visible) > 0 {
		fmt.Fprintf(w, "\nAvailable commands:\n")
		maxLen := 0
		for _, name := range visible {
			if len(name) > maxLen {
				maxLen = len(name)
			}
		}
		for _, name := range visible {
			child := g.Children[name]
			fmt.Fprintf(w, "  %-*s  %s\n", maxLen, name, child.Summary)
		}
	}

	if g.FlagSet != nil {
		writeFlags(w, g.FlagSet)
	}

	fmt.Fprintf(w, "\nUse \"%s <command> --help\" for more information.\n", pathStr)
}

// writeCommandHelp writes help text for a leaf command to w.
// path is the full command path (e.g. ["app", "repo", "pr", "list"]).
// fs is the command's flag set (may be nil).
func writeCommandHelp(w io.Writer, cmd *Node, path []string, fs *flag.FlagSet) {
	pathStr := strings.Join(path, " ")
	fmt.Fprintf(w, "Usage: %s [flags]\n", pathStr)

	if cmd.Summary != "" {
		fmt.Fprintf(w, "\n%s\n", cmd.Summary)
	}

	if fs != nil {
		writeFlags(w, fs)
	}
}

func visibleChildren(g *Node) []string {
	names := make([]string, 0, len(g.Children))
	for name, child := range g.Children {
		if !child.Hidden {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func writeFlags(w io.Writer, fs *flag.FlagSet) {
	var flags []string
	fs.VisitAll(func(f *flag.Flag) {
		flags = append(flags, fmt.Sprintf("  --%s %s\n    \t%s", f.Name, f.DefValue, f.Usage))
	})
	if len(flags) > 0 {
		fmt.Fprintf(w, "\nFlags:\n")
		for _, f := range flags {
			fmt.Fprintln(w, f)
		}
	}
}
