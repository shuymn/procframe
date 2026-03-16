package codegen

import "slices"

// treeNode is the intermediate representation of a CLI command tree node,
// built from proto descriptors before code generation.
type treeNode struct {
	Segment  string
	Summary  string
	Hidden   bool
	BindInto string               // non-empty for groups with bind_into
	Children map[string]*treeNode // non-nil for groups
	Leaf     *leafInfo            // non-nil for leaves
}

// leafInfo holds proto metadata for a leaf command (an RPC method).
type leafInfo struct {
	ServiceGoName string // Go name of the service (e.g. "RepoPRService")
	MethodGoName  string // Go name of the method (e.g. "List")
	InputGoName   string // Go name of the request type (e.g. "PullRequestListRequest")
	OutputGoName  string // Go name of the response type (e.g. "PullRequestListResponse")
	FullName      string // full protobuf procedure name (e.g. "/pkg.v1.RepoPRService/List")
	IsStreaming   bool   // true for server-streaming methods
}

func (n *treeNode) isGroup() bool {
	return n.Children != nil
}

// buildTree constructs a treeNode from the given service descriptors.
func buildTree(services []serviceInfo) *treeNode {
	root := &treeNode{Children: make(map[string]*treeNode)}

	for i := range services {
		svc := &services[i]
		for j := range svc.Methods {
			m := &svc.Methods[j]
			if !m.CLI {
				continue
			}
			path := slices.Concat(svc.Path, m.Path)
			insertLeaf(root, path, &leafInfo{
				ServiceGoName: svc.GoName,
				MethodGoName:  m.GoName,
				InputGoName:   m.InputGoName,
				OutputGoName:  m.OutputGoName,
				FullName:      m.FullName,
				IsStreaming:   m.IsStreaming,
			}, svc)
		}
	}

	pruneDeadGroups(root)
	return root
}

func insertLeaf(root *treeNode, path []string, leaf *leafInfo, svc *serviceInfo) {
	node := root
	svcPathLen := len(svc.Path)

	for i, seg := range path {
		child, ok := node.Children[seg]
		if !ok {
			child = &treeNode{
				Segment:  seg,
				Children: make(map[string]*treeNode),
			}
			node.Children[seg] = child
		}
		// Apply service-level options at the last segment of the service path
		if i == svcPathLen-1 && svcPathLen > 0 {
			child.Summary = svc.Summary
			child.Hidden = svc.Hidden
			child.BindInto = svc.BindInto
		}
		node = child
	}

	// Convert the terminal group node to a leaf
	node.Children = nil
	node.Leaf = leaf
	node.Summary = leaf.summary(svc)
}

func (l *leafInfo) summary(svc *serviceInfo) string {
	for _, m := range svc.Methods {
		if m.GoName == l.MethodGoName {
			return m.Summary
		}
	}
	return ""
}

// pruneDeadGroups removes group nodes that have no leaf descendants.
func pruneDeadGroups(node *treeNode) {
	if !node.isGroup() {
		return
	}
	for name, child := range node.Children {
		pruneDeadGroups(child)
		if child.isGroup() && len(child.Children) == 0 {
			delete(node.Children, name)
		}
	}
}

// serviceInfo holds extracted information from a protobuf service descriptor.
type serviceInfo struct {
	GoName   string
	Path     []string // from cli_group.path segments
	BindInto string
	Summary  string
	Hidden   bool
	Methods  []methodInfo
}

// methodInfo holds extracted information from a protobuf method descriptor.
type methodInfo struct {
	GoName       string
	Path         []string // from proc.cli_path segments
	CLI          bool
	Connect      bool
	Summary      string
	Hidden       bool
	InputGoName  string
	OutputGoName string
	FullName     string
	IsStreaming  bool
}
