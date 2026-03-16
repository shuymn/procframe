package codegen

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var (
	base64Pkg       = protogen.GoImportPath("encoding/base64")
	contextPkg      = protogen.GoImportPath("context")
	encodingJSONPkg = protogen.GoImportPath("encoding/json")
	flagPkg         = protogen.GoImportPath("flag")
	fmtPkg          = protogen.GoImportPath("fmt")
	ioPkg           = protogen.GoImportPath("io")
	stringsPkg      = protogen.GoImportPath("strings")
	protojsonPkg    = protogen.GoImportPath("google.golang.org/protobuf/encoding/protojson")
	procframePkg    = protogen.GoImportPath("github.com/shuymn/procframe")
	cliPkg          = protogen.GoImportPath("github.com/shuymn/procframe/transport/cli")
)

// generateCLI writes the CLI runner for a service.
func generateCLI(
	g *protogen.GeneratedFile,
	svc *protogen.Service,
	svcInfo *serviceInfo,
	tree *treeNode,
) {
	handlerType := svc.GoName + "Handler"
	schemaEntries := collectSchemaEntries(svc, svcInfo)
	schemaNames := schemaDataNames(svc.GoName)

	// Collect all nodes bottom-up and assign to variables
	var decls []nodeDecl
	collectDecls(tree, nil, svc, svcInfo, &decls)

	// Resolve bind_into contexts
	bindCtxs := resolveBindIntoCtxs(tree, svc)

	emitSchemaDataDecls(g, schemaEntries, schemaNames)

	g.P("// New", svc.GoName, "CLIRunner constructs a [", cliPkg.Ident("Runner"), "]")
	g.P("// for ", svc.GoName, ".")
	g.P(
		"func New",
		svc.GoName,
		"CLIRunner(h ",
		handlerType,
		", opts ...",
		cliPkg.Ident("Option"),
		") *",
		cliPkg.Ident("Runner"),
		" {",
	)

	// Emit bind_into group flag variable declarations
	for _, bc := range bindCtxs {
		emitBindIntoVars(g, bc)
	}

	// Emit leaf run functions first, then group nodes bottom-up
	for _, d := range decls {
		if d.node.Leaf != nil {
			emitLeafDecl(g, d, svc, bindCtxs)
		} else {
			emitGroupDecl(g, d, bindCtxs)
		}
	}

	rootVar := decls[len(decls)-1].varName

	// Add schema subcommand to root
	emitSchemaNode(g, rootVar, schemaNames)

	g.P("\treturn ", cliPkg.Ident("NewRunner"), "(", rootVar, ", opts...)")
	g.P("}")
}

// nodeDecl pairs a tree node with the variable name assigned to it.
type nodeDecl struct {
	node    *treeNode
	varName string
	path    []string // path segments leading to this node
}

// bindIntoCtx holds resolved bind_into information for code generation.
type bindIntoCtx struct {
	fieldName   string            // proto field name in request (e.g. "pr")
	goFieldName string            // Go field name (e.g. "Pr")
	msg         *protogen.Message // the bind_into message type
	varPrefix   string            // variable prefix (e.g. "bind_pr")
}

type schemaEntry struct {
	commandPath string
	procedure   string
	summary     string
	streaming   bool
	input       *protogen.Message
	output      *protogen.Message
}

type schemaDataVarNames struct {
	byCommand   string
	byProcedure string
	all         string
}

// collectDecls walks the tree depth-first and collects declarations
// in bottom-up order (leaves first, root last).
func collectDecls(node *treeNode, path []string, svc *protogen.Service, svcInfo *serviceInfo, decls *[]nodeDecl) {
	if node.Leaf != nil {
		*decls = append(*decls, nodeDecl{
			node:    node,
			varName: pathToVarName(path),
			path:    path,
		})
		return
	}

	names := sortedKeys(node.Children)
	for _, name := range names {
		childPath := append(append([]string{}, path...), name)
		collectDecls(node.Children[name], childPath, svc, svcInfo, decls)
	}

	*decls = append(*decls, nodeDecl{
		node:    node,
		varName: pathToVarName(path),
		path:    path,
	})
}

// sortedKeys returns the keys of a map[string]*treeNode in sorted order.
func sortedKeys(m map[string]*treeNode) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func pathToVarName(path []string) string {
	if len(path) == 0 {
		return "root"
	}
	parts := make([]string, len(path))
	for i, p := range path {
		parts[i] = strings.ReplaceAll(p, "-", "_")
	}
	return "node_" + strings.Join(parts, "_")
}

func collectSchemaEntries(svc *protogen.Service, svcInfo *serviceInfo) []schemaEntry {
	entries := make([]schemaEntry, 0, len(svcInfo.Methods))
	for _, mi := range svcInfo.Methods {
		if !mi.CLI {
			continue
		}
		method := findMethod(svc, mi.GoName)
		if method == nil {
			continue
		}
		entries = append(entries, schemaEntry{
			commandPath: strings.Join(slices.Concat(svcInfo.Path, mi.Path), " "),
			procedure:   mi.FullName,
			summary:     mi.Summary,
			streaming:   mi.IsStreaming,
			input:       method.Input,
			output:      method.Output,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].commandPath < entries[j].commandPath
	})
	return entries
}

func schemaDataNames(serviceGoName string) schemaDataVarNames {
	base := "schemaData" + serviceGoName
	return schemaDataVarNames{
		byCommand:   base + "ByCommand",
		byProcedure: base + "ByProcedure",
		all:         base + "All",
	}
}

func emitGroupDecl(g *protogen.GeneratedFile, d nodeDecl, bindCtxs []bindIntoCtx) {
	g.P("\t", d.varName, " := &", cliPkg.Ident("Node"), "{")
	if d.node.Segment != "" {
		g.P("\t\tSegment: ", fmt.Sprintf("%q", d.node.Segment), ",")
	}
	if d.node.Summary != "" {
		g.P("\t\tSummary: ", fmt.Sprintf("%q", d.node.Summary), ",")
	}
	if d.node.Hidden {
		g.P("\t\tHidden: true,")
	}
	if d.node.BindInto != "" {
		bc := findBindCtx(bindCtxs, d.node.BindInto)
		emitGroupFlagSet(g, d, bc)
	}
	g.P("\t\tChildren: map[string]*", cliPkg.Ident("Node"), "{")
	childNames := sortedKeys(d.node.Children)
	for _, name := range childNames {
		childPath := append(append([]string{}, d.path...), name)
		childVar := pathToVarName(childPath)
		g.P("\t\t\t", fmt.Sprintf("%q", name), ": ", childVar, ",")
	}
	g.P("\t\t},")
	g.P("\t}")
}

func emitGroupFlagSet(g *protogen.GeneratedFile, d nodeDecl, bc *bindIntoCtx) {
	g.P("\t\tFlagSet: func() *", flagPkg.Ident("FlagSet"), " {")
	g.P(
		"\t\t\tfs := ",
		flagPkg.Ident("NewFlagSet"),
		"(",
		fmt.Sprintf("%q", d.node.Segment),
		", ",
		flagPkg.Ident("ContinueOnError"),
		")",
	)
	g.P("\t\t\tfs.SetOutput(", ioPkg.Ident("Discard"), ")")
	if bc != nil {
		emitBindIntoFlagRegistration(g, bc, "\t\t\t")
	}
	g.P("\t\t\treturn fs")
	g.P("\t\t}(),")
}

func emitLeafDecl(g *protogen.GeneratedFile, d nodeDecl, svc *protogen.Service, bindCtxs []bindIntoCtx) {
	leaf := d.node.Leaf

	g.P("\t", d.varName, " := &", cliPkg.Ident("Node"), "{")
	g.P("\t\tSegment: ", fmt.Sprintf("%q", d.node.Segment), ",")
	if d.node.Summary != "" {
		g.P("\t\tSummary: ", fmt.Sprintf("%q", d.node.Summary), ",")
	}
	if d.node.Hidden {
		g.P("\t\tHidden: true,")
	}

	method := findMethod(svc, leaf.MethodGoName)
	if method == nil {
		g.P("\t}")
		return
	}

	g.P(
		"\t\tRun: func(ctx ",
		contextPkg.Ident("Context"),
		", args []string, stdout ",
		ioPkg.Ident("Writer"),
		") error {",
	)

	// --json / flag parsing branching
	emitRequestParsing(g, d, method, bindCtxs)

	if leaf.IsStreaming {
		emitStreamingHandlerCall(g, method, leaf)
	} else {
		emitHandlerCall(g, method, leaf)
	}

	g.P("\t\t},")

	// HelpFlags factory for help display
	emitHelpFlags(g, method.Input, bindCtxs)

	g.P("\t}")
}

// emitHelpFlags emits a HelpFlags factory function on the node
// that returns a FlagSet with flag definitions for help display.
func emitHelpFlags(g *protogen.GeneratedFile, msg *protogen.Message, bindCtxs []bindIntoCtx) {
	g.P("\t\tHelpFlags: func() *", flagPkg.Ident("FlagSet"), " {")
	g.P(
		"\t\t\tfs := ",
		flagPkg.Ident("NewFlagSet"),
		`("", `,
		flagPkg.Ident("ContinueOnError"),
		")",
	)
	for _, field := range msg.Fields {
		protoName := string(field.Desc.Name())
		if findBindCtx(bindCtxs, protoName) != nil {
			continue
		}
		emitHelpFlagReg(g, field, "\t\t\t")
	}
	g.P("\t\t\treturn fs")
	g.P("\t\t},")
}

// emitHelpFlagReg registers a single flag on the help FlagSet.
// Uses the same names/types/usage as the real flags but without variable bindings.
func emitHelpFlagReg(g *protogen.GeneratedFile, field *protogen.Field, indent string) {
	usage := fieldUsage(field)
	flagName := fieldToFlagName(string(field.Desc.Name()))
	qName := fmt.Sprintf("%q", flagName)
	qUsage := fmt.Sprintf("%q", usage)

	// Map fields → string flag for help
	if field.Desc.IsMap() {
		g.P(indent, "fs.String(", qName, ", ", `""`, ", ", qUsage, ")")
		return
	}

	// Repeated fields
	if field.Desc.IsList() {
		emitHelpRepeatedFlagReg(g, field, indent, qName, qUsage)
		return
	}

	// Singular enum
	if field.Desc.Kind() == protoreflect.EnumKind {
		emitHelpEnumFlagReg(g, field, indent, qName, qUsage)
		return
	}

	// Singular scalar/message/bytes
	ti := resolveFieldType(field)
	if ti == nil {
		return
	}
	if ti.stdFlagFunc != "" {
		stdFunc := strings.TrimSuffix(ti.stdFlagFunc, "Var")
		g.P(indent, "fs.", stdFunc, "(", qName, ", ", ti.defaultVal, ", ", qUsage, ")")
	} else {
		g.P(indent, "fs.Var(", cliPkg.Ident(ti.constructor), "(nil), ", qName, ", ", qUsage, ")")
	}
}

func emitHelpRepeatedFlagReg(
	g *protogen.GeneratedFile,
	field *protogen.Field,
	indent, qName, qUsage string,
) {
	switch field.Desc.Kind() {
	case protoreflect.MessageKind, protoreflect.BytesKind, protoreflect.GroupKind:
		g.P(indent, "fs.String(", qName, ", ", `""`, ", ", qUsage, ")")
	case protoreflect.EnumKind:
		typeName := string(field.Enum.Desc.Name())
		g.P(indent, "fs.Var(", cliPkg.Ident("NewEnumSliceValue"),
			"(nil, []", cliPkg.Ident("EnumMapping"), "{")
		emitEnumMappingEntries(g, field.Enum, indent)
		g.P(indent, "}, ", fmt.Sprintf("%q", typeName), "), ", qName, ", ", qUsage, ")")
	case protoreflect.StringKind:
		g.P(indent, "fs.Var(", cliPkg.Ident("NewStringSliceValue"), "(nil), ", qName, ", ", qUsage, ")")
	case protoreflect.BoolKind,
		protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind,
		protoreflect.FloatKind, protoreflect.DoubleKind:
		ri := resolveRepeatedFieldType(field)
		if ri != nil {
			g.P(indent, "fs.Var(", cliPkg.Ident(ri.constructor), "(nil), ", qName, ", ", qUsage, ")")
		}
	}
}

func emitHelpEnumFlagReg(
	g *protogen.GeneratedFile,
	field *protogen.Field,
	indent, qName, qUsage string,
) {
	typeName := string(field.Enum.Desc.Name())
	g.P(indent, "fs.Var(", cliPkg.Ident("NewEnumValue"),
		"(nil, []", cliPkg.Ident("EnumMapping"), "{")
	emitEnumMappingEntries(g, field.Enum, indent)
	g.P(indent, "}, ", fmt.Sprintf("%q", typeName), "), ", qName, ", ", qUsage, ")")
}

// emitRequestParsing emits the --json / flag parsing branching.
func emitRequestParsing(
	g *protogen.GeneratedFile,
	d nodeDecl,
	method *protogen.Method,
	bindCtxs []bindIntoCtx,
) {
	g.P("\t\t\tvar req *", method.Input.GoIdent)
	g.P("\t\t\tif jsonPayload, ok := ", cliPkg.Ident("JSONPayloadFromContext"), "(ctx); ok {")
	g.P("\t\t\t\tif len(args) > 0 {")
	g.P(
		"\t\t\t\t\treturn ",
		procframePkg.Ident("NewError"),
		"(",
		procframePkg.Ident("CodeInvalidArgument"),
		`, "--json cannot be combined with flags")`,
	)
	g.P("\t\t\t\t}")
	g.P("\t\t\t\treq = &", method.Input.GoIdent, "{}")
	g.P("\t\t\t\tif err := ", protojsonPkg.Ident("Unmarshal"), "([]byte(jsonPayload), req); err != nil {")
	g.P("\t\t\t\t\treturn err")
	g.P("\t\t\t\t}")
	g.P("\t\t\t} else {")

	// Flag parsing branch
	g.P(
		"\t\t\t\tfs := ",
		flagPkg.Ident("NewFlagSet"),
		"(",
		fmt.Sprintf("%q", d.node.Segment),
		", ",
		flagPkg.Ident("ContinueOnError"),
		")",
	)
	g.P("\t\t\t\tfs.SetOutput(", ioPkg.Ident("Discard"), ")")
	emitFlagVars(g, method.Input, "\t\t\t\t", bindCtxs)
	g.P("\t\t\t\tif err := fs.Parse(args); err != nil {")
	g.P("\t\t\t\t\treturn err")
	g.P("\t\t\t\t}")
	g.P("\t\t\t\treq = &", method.Input.GoIdent, "{")
	emitRequestFields(g, method.Input, "\t\t\t\t\t", bindCtxs)
	g.P("\t\t\t\t}")
	emitRequestPostAssign(g, method.Input, "\t\t\t\t", bindCtxs)
	g.P("\t\t\t}")
}

// emitHandlerCall emits the unary handler invocation and JSON output code
// within a leaf command's Run function.
func emitHandlerCall(g *protogen.GeneratedFile, method *protogen.Method, leaf *leafInfo) {
	g.P(
		"\t\t\tresp, err := ",
		procframePkg.Ident("InvokeUnary"),
		"(ctx, ",
		procframePkg.Ident("CallSpec"),
		"{",
		"Procedure: ",
		fmt.Sprintf("%q", leaf.FullName),
		", Transport: ",
		procframePkg.Ident("TransportCLI"),
		", StreamType: ",
		procframePkg.Ident("StreamTypeUnary"),
		"}, &",
		procframePkg.Ident("Request"),
		"[",
		method.Input.GoIdent,
		"]{",
	)
	g.P("\t\t\t\tMsg:  req,")
	g.P("\t\t\t\tMeta: ", procframePkg.Ident("Meta"), "{Procedure: ", fmt.Sprintf("%q", leaf.FullName), "},")
	g.P("\t\t\t}, h.", method.GoName, ", ", cliPkg.Ident("InterceptorsFromContext"), "(ctx)...)")
	g.P("\t\t\tif err != nil {")
	g.P("\t\t\t\treturn err")
	g.P("\t\t\t}")
	g.P("\t\t\tif resp == nil || resp.Msg == nil {")
	g.P(
		"\t\t\t\treturn ",
		procframePkg.Ident("NewError"),
		"(",
		procframePkg.Ident("CodeInternal"),
		`, "handler returned nil response")`,
	)
	g.P("\t\t\t}")

	// --output json: compact JSON; default: multiline JSON
	g.P("\t\t\tvar out []byte")
	g.P("\t\t\tif ", cliPkg.Ident("OutputFormatFromContext"), "(ctx) == ", cliPkg.Ident("OutputJSON"), " {")
	g.P("\t\t\t\tout, err = ", protojsonPkg.Ident("MarshalOptions"), "{}.Marshal(resp.Msg)")
	g.P("\t\t\t} else {")
	g.P("\t\t\t\tout, err = ", protojsonPkg.Ident("MarshalOptions"), "{Multiline: true}.Marshal(resp.Msg)")
	g.P("\t\t\t}")
	g.P("\t\t\tif err != nil {")
	g.P("\t\t\t\treturn err")
	g.P("\t\t\t}")
	g.P("\t\t\t", fmtPkg.Ident("Fprintln"), "(stdout, string(out))")
	g.P("\t\t\treturn nil")
}

// emitStreamingHandlerCall emits the server-streaming handler invocation.
func emitStreamingHandlerCall(g *protogen.GeneratedFile, method *protogen.Method, leaf *leafInfo) {
	// Build write callback
	g.P(
		"\t\t\tstream := ",
		cliPkg.Ident("NewStreamWriter"),
		"[", method.Output.GoIdent, "](",
		"ctx, func(resp *",
		procframePkg.Ident("Response"),
		"[", method.Output.GoIdent, "]) error {",
	)
	g.P("\t\t\t\tif resp == nil || resp.Msg == nil {")
	g.P(
		"\t\t\t\t\treturn ",
		procframePkg.Ident("NewError"),
		"(",
		procframePkg.Ident("CodeInternal"),
		`, "handler sent nil response")`,
	)
	g.P("\t\t\t\t}")
	g.P("\t\t\t\tvar out []byte")
	g.P("\t\t\t\tvar err error")
	g.P("\t\t\t\tif ", cliPkg.Ident("OutputFormatFromContext"), "(ctx) == ", cliPkg.Ident("OutputJSON"), " {")
	g.P("\t\t\t\t\tout, err = ", protojsonPkg.Ident("MarshalOptions"), "{}.Marshal(resp.Msg)")
	g.P("\t\t\t\t} else {")
	g.P("\t\t\t\t\tout, err = ", protojsonPkg.Ident("MarshalOptions"), "{Multiline: true}.Marshal(resp.Msg)")
	g.P("\t\t\t\t}")
	g.P("\t\t\t\tif err != nil {")
	g.P("\t\t\t\t\treturn err")
	g.P("\t\t\t\t}")
	g.P("\t\t\t\t", fmtPkg.Ident("Fprintln"), "(stdout, string(out))")
	g.P("\t\t\t\treturn nil")
	g.P("\t\t\t})")

	// Invoke streaming handler
	g.P(
		"\t\t\treturn ",
		procframePkg.Ident("InvokeServerStream"),
		"(ctx, ",
		procframePkg.Ident("CallSpec"),
		"{",
		"Procedure: ",
		fmt.Sprintf("%q", leaf.FullName),
		", Transport: ",
		procframePkg.Ident("TransportCLI"),
		", StreamType: ",
		procframePkg.Ident("StreamTypeServerStream"),
		"}, &",
		procframePkg.Ident("Request"),
		"[",
		method.Input.GoIdent,
		"]{",
	)
	g.P("\t\t\t\tMsg:  req,")
	g.P("\t\t\t\tMeta: ", procframePkg.Ident("Meta"), "{Procedure: ", fmt.Sprintf("%q", leaf.FullName), "},")
	g.P("\t\t\t}, stream, h.", method.GoName, ", ", cliPkg.Ident("InterceptorsFromContext"), "(ctx)...)")
}

func emitFlagVars(
	g *protogen.GeneratedFile,
	msg *protogen.Message,
	indent string,
	bindCtxs []bindIntoCtx,
) {
	for _, field := range msg.Fields {
		protoName := string(field.Desc.Name())
		if findBindCtx(bindCtxs, protoName) != nil {
			continue
		}
		emitFlagVar(g, field, indent)
	}
}

// fieldUsage builds a flag usage string from the field's description option
// and, for enum fields, appends allowed values.
func fieldUsage(field *protogen.Field) string {
	var parts []string
	if desc := getFieldDescription(field); desc != "" {
		parts = append(parts, desc)
	}
	if field.Desc.Kind() == protoreflect.EnumKind {
		values := enumCLIValuesForSchema(field.Enum)
		if len(values) > 0 {
			parts = append(parts, "(values: "+strings.Join(values, ", ")+")")
		}
	}
	if isJSONStringFlag(field) {
		parts = append(parts, "(JSON)")
	}
	if !field.Desc.IsList() && field.Desc.Kind() == protoreflect.BytesKind {
		parts = append(parts, "(base64)")
	}
	return strings.Join(parts, " ")
}

// isJSONStringFlag returns true if the field is exposed as a JSON string flag.
func isJSONStringFlag(field *protogen.Field) bool {
	kind := field.Desc.Kind()
	if field.Desc.IsMap() {
		return true
	}
	if !field.Desc.IsList() &&
		(kind == protoreflect.MessageKind || kind == protoreflect.GroupKind) {
		return true
	}
	if field.Desc.IsList() &&
		(kind == protoreflect.MessageKind ||
			kind == protoreflect.BytesKind ||
			kind == protoreflect.GroupKind) {
		return true
	}
	return false
}

func emitFlagVar(g *protogen.GeneratedFile, field *protogen.Field, indent string) {
	usage := fieldUsage(field)
	name := fieldToFlagName(string(field.Desc.Name()))
	varName := "flag_" + string(field.Desc.Name())

	// Map fields → JSON string flag
	if field.Desc.IsMap() {
		g.P(indent, "var ", varName, " string")
		g.P(
			indent, "fs.StringVar(&", varName, ", ",
			fmt.Sprintf("%q", name), ", ", `""`, ", ",
			fmt.Sprintf("%q", usage), ")",
		)
		return
	}

	// Repeated fields
	if field.Desc.IsList() {
		switch field.Desc.Kind() {
		case protoreflect.MessageKind, protoreflect.BytesKind, protoreflect.GroupKind:
			// JSON string flag
			g.P(indent, "var ", varName, " string")
			g.P(
				indent, "fs.StringVar(&", varName, ", ",
				fmt.Sprintf("%q", name), ", ", `""`, ", ",
				fmt.Sprintf("%q", usage), ")",
			)
		case protoreflect.EnumKind:
			emitEnumSliceFlagVar(g, field, indent, usage)
		case protoreflect.StringKind:
			g.P(indent, "var ", varName, " []string")
			emitFsVar(g, indent, "NewStringSliceValue", varName, name, usage)
		case protoreflect.BoolKind,
			protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
			protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
			protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
			protoreflect.Uint64Kind, protoreflect.Fixed64Kind,
			protoreflect.FloatKind, protoreflect.DoubleKind:
			ri := resolveRepeatedFieldType(field)
			if ri == nil {
				return
			}
			g.P(indent, "var ", varName, " ", ri.goType)
			emitFsVar(g, indent, ri.constructor, varName, name, usage)
		}
		return
	}

	// Singular enum
	if field.Desc.Kind() == protoreflect.EnumKind {
		emitEnumFlagVar(g, field, indent, usage)
		return
	}

	// Singular scalar/message/bytes
	ti := resolveFieldType(field)
	if ti == nil {
		return
	}

	g.P(indent, "var ", varName, " ", ti.goType)
	emitFlagReg(g, indent, ti, varName, name, usage)
}

// repeatedFieldTypeInfo maps a repeated field kind to Go slice type and constructor.
type repeatedFieldTypeInfo struct {
	goType      string
	constructor string
}

func resolveRepeatedFieldType(field *protogen.Field) *repeatedFieldTypeInfo {
	switch field.Desc.Kind() {
	case protoreflect.BoolKind:
		return &repeatedFieldTypeInfo{goType: "[]bool", constructor: "NewBoolSliceValue"}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return &repeatedFieldTypeInfo{goType: "[]int32", constructor: "NewInt32SliceValue"}
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return &repeatedFieldTypeInfo{goType: "[]int64", constructor: "NewInt64SliceValue"}
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return &repeatedFieldTypeInfo{goType: "[]uint32", constructor: "NewUint32SliceValue"}
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return &repeatedFieldTypeInfo{goType: "[]uint64", constructor: "NewUint64SliceValue"}
	case protoreflect.FloatKind:
		return &repeatedFieldTypeInfo{goType: "[]float32", constructor: "NewFloat32SliceValue"}
	case protoreflect.DoubleKind:
		return &repeatedFieldTypeInfo{goType: "[]float64", constructor: "NewFloat64SliceValue"}
	case protoreflect.EnumKind, protoreflect.StringKind,
		protoreflect.BytesKind, protoreflect.MessageKind, protoreflect.GroupKind:
		return nil
	default:
		return nil
	}
}

func emitEnumSliceFlagVar(g *protogen.GeneratedFile, field *protogen.Field, indent, usage string) {
	name := fieldToFlagName(string(field.Desc.Name()))
	varName := "flag_" + string(field.Desc.Name())
	typeName := string(field.Enum.Desc.Name())

	g.P(indent, "var ", varName, " []int32")
	g.P(indent, "fs.Var(", cliPkg.Ident("NewEnumSliceValue"), "(&", varName, ", []", cliPkg.Ident("EnumMapping"), "{")
	emitEnumMappingEntries(g, field.Enum, indent)
	g.P(indent, "}, ", fmt.Sprintf("%q", typeName), "), ", fmt.Sprintf("%q", name), ", ", fmt.Sprintf("%q", usage), ")")
}

func emitFsVar(g *protogen.GeneratedFile, indent, constructor, varName, flagName, usage string) {
	g.P(
		indent,
		"fs.Var(",
		cliPkg.Ident(constructor),
		"(&",
		varName,
		"), ",
		fmt.Sprintf("%q", flagName),
		", ",
		fmt.Sprintf("%q", usage),
		")",
	)
}

// fieldTypeInfo maps a protobuf field kind to Go type and flag registration info.
type fieldTypeInfo struct {
	goType      string // Go type name (e.g. "int32")
	stdFlagFunc string // stdlib flag func (e.g. "StringVar"), empty if custom
	constructor string // cli.NewXxxValue constructor, empty if stdlib
	defaultVal  string // default value for stdlib flag funcs
}

func resolveFieldType(field *protogen.Field) *fieldTypeInfo {
	switch field.Desc.Kind() {
	case protoreflect.StringKind:
		return &fieldTypeInfo{goType: "string", stdFlagFunc: "StringVar", defaultVal: `""`}
	case protoreflect.BoolKind:
		return &fieldTypeInfo{goType: "bool", stdFlagFunc: "BoolVar", defaultVal: "false"}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return &fieldTypeInfo{goType: "int32", constructor: "NewInt32Value"}
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return &fieldTypeInfo{goType: "int64", constructor: "NewInt64Value"}
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return &fieldTypeInfo{goType: "uint32", constructor: "NewUint32Value"}
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return &fieldTypeInfo{goType: "uint64", constructor: "NewUint64Value"}
	case protoreflect.FloatKind:
		return &fieldTypeInfo{goType: "float32", constructor: "NewFloat32Value"}
	case protoreflect.DoubleKind:
		return &fieldTypeInfo{goType: "float64", stdFlagFunc: "Float64Var", defaultVal: "0"}
	case protoreflect.BytesKind:
		return &fieldTypeInfo{goType: "string", stdFlagFunc: "StringVar", defaultVal: `""`}
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return &fieldTypeInfo{goType: "string", stdFlagFunc: "StringVar", defaultVal: `""`}
	case protoreflect.EnumKind:
		return nil
	default:
		return nil
	}
}

func emitFlagReg(g *protogen.GeneratedFile, indent string, ti *fieldTypeInfo, varName, flagName, usage string) {
	if ti.stdFlagFunc != "" {
		g.P(
			indent,
			"fs.",
			ti.stdFlagFunc,
			"(&",
			varName,
			", ",
			fmt.Sprintf("%q", flagName),
			", ",
			ti.defaultVal,
			", ",
			fmt.Sprintf("%q", usage),
			")",
		)
	} else {
		emitFsVar(g, indent, ti.constructor, varName, flagName, usage)
	}
}

func emitEnumFlagVar(g *protogen.GeneratedFile, field *protogen.Field, indent, usage string) {
	name := fieldToFlagName(string(field.Desc.Name()))
	varName := "flag_" + string(field.Desc.Name())
	typeName := string(field.Enum.Desc.Name())

	g.P(indent, "var ", varName, " int32")
	g.P(indent, "fs.Var(", cliPkg.Ident("NewEnumValue"), "(&", varName, ", []", cliPkg.Ident("EnumMapping"), "{")
	emitEnumMappingEntries(g, field.Enum, indent)
	g.P(indent, "}, ", fmt.Sprintf("%q", typeName), "), ", fmt.Sprintf("%q", name), ", ", fmt.Sprintf("%q", usage), ")")
}

// emitEnumMappingEntries emits []cli.EnumMapping literal entries
// using enumCLIValues to ensure consistent transform with validation.
func emitEnumMappingEntries(g *protogen.GeneratedFile, enumType *protogen.Enum, indent string) {
	// enumCLIValues error is impossible here: validateEnumCollisions
	// already ran.
	mappings, err := enumCLIValues(string(enumType.Desc.Name()), extractEnumValueInfos(enumType))
	if err != nil {
		return
	}
	for _, m := range mappings {
		g.P(indent, "\t{CLIValue: ", fmt.Sprintf("%q", m.CLIValue), ", Number: ", m.Number, "},")
	}
}

func emitRequestFields(
	g *protogen.GeneratedFile,
	msg *protogen.Message,
	indent string,
	bindCtxs []bindIntoCtx,
) {
	for _, field := range msg.Fields {
		varName := "flag_" + string(field.Desc.Name())
		protoName := string(field.Desc.Name())

		if bc := findBindCtx(bindCtxs, protoName); bc != nil {
			emitBindIntoFieldConstruction(g, bc, indent)
			continue
		}

		// Skip fields that need post-assignment (JSON string fields, bytes)
		if isPostAssignField(field) {
			continue
		}

		emitFieldAssignment(g, field, varName, indent)
	}
}

// emitFieldAssignment emits a single struct literal field assignment,
// handling enum casts and repeated enum IIFE conversions.
func emitFieldAssignment(
	g *protogen.GeneratedFile,
	field *protogen.Field,
	varName, indent string,
) {
	switch {
	case field.Desc.IsList() && field.Desc.Kind() == protoreflect.EnumKind:
		// repeated enum: []int32 → []EnumType conversion via IIFE
		g.P(indent, field.GoName, ": func() []", field.Enum.GoIdent, " {")
		g.P(indent, "\tout := make([]", field.Enum.GoIdent, ", len(", varName, "))")
		g.P(indent, "\tfor i, v := range ", varName, " { out[i] = ", field.Enum.GoIdent, "(v) }")
		g.P(indent, "\treturn out")
		g.P(indent, "}(),")
	case field.Desc.Kind() == protoreflect.EnumKind:
		g.P(indent, field.GoName, ": ", field.Enum.GoIdent, "(", varName, "),")
	default:
		g.P(indent, field.GoName, ": ", varName, ",")
	}
}

// isPostAssignField returns true for fields that require post-assignment
// instead of struct literal inclusion. This includes all JSON string flag
// fields plus singular bytes (base64-decoded post-assignment).
func isPostAssignField(field *protogen.Field) bool {
	if isJSONStringFlag(field) {
		return true
	}
	// Singular bytes → base64 flag (not JSON, but still post-assign)
	return !field.Desc.IsList() && field.Desc.Kind() == protoreflect.BytesKind
}

// emitRequestPostAssign emits post-assignment code for JSON string fields,
// bytes fields, and complex bind_into fields after the struct literal.
func emitRequestPostAssign(
	g *protogen.GeneratedFile,
	msg *protogen.Message,
	indent string,
	bindCtxs []bindIntoCtx,
) {
	// Handle complex bind_into fields
	for _, field := range msg.Fields {
		protoName := string(field.Desc.Name())
		bc := findBindCtx(bindCtxs, protoName)
		if bc == nil {
			continue
		}
		if !hasBindIntoComplexFields(bc) {
			continue
		}
		emitBindIntoPostAssign(g, bc, indent)
	}

	// Handle direct post-assign fields
	for _, field := range msg.Fields {
		protoName := string(field.Desc.Name())
		if findBindCtx(bindCtxs, protoName) != nil {
			continue
		}
		if !isPostAssignField(field) {
			continue
		}
		varName := "flag_" + string(field.Desc.Name())
		emitPostAssignFieldCode(g, field, varName, "req", indent)
	}
}

// emitPostAssignFieldCode emits the post-assignment code for a single
// complex field: base64 decode for bytes, JSON unmarshal for everything else.
func emitPostAssignFieldCode(
	g *protogen.GeneratedFile,
	field *protogen.Field,
	varName, targetExpr, indent string,
) {
	jsonName := field.Desc.JSONName()
	if !field.Desc.IsList() && field.Desc.Kind() == protoreflect.BytesKind {
		g.P(indent, "if ", varName, ` != "" {`)
		g.P(indent, "\t", varName, "Bytes, err := ", base64Pkg.Ident("StdEncoding"), ".DecodeString(", varName, ")")
		g.P(indent, "\tif err != nil {")
		g.P(indent, "\t\treturn err")
		g.P(indent, "\t}")
		g.P(indent, "\t", targetExpr, ".", field.GoName, " = ", varName, "Bytes")
		g.P(indent, "}")
	} else {
		g.P(indent, "if ", varName, ` != "" {`)
		g.P(
			indent, "\tif err := ",
			cliPkg.Ident("UnmarshalJSONField"),
			"(", targetExpr, ", ", fmt.Sprintf("%q", jsonName), ", ", varName,
			"); err != nil {",
		)
		g.P(indent, "\t\treturn err")
		g.P(indent, "\t}")
		g.P(indent, "}")
	}
}

// emitBindIntoPostAssign emits the post-assignment construction of a
// bind_into message that has complex fields.
func emitBindIntoPostAssign(g *protogen.GeneratedFile, bc *bindIntoCtx, indent string) {
	tmpVar := bc.varPrefix + "Msg"
	g.P(indent, tmpVar, " := &", bc.msg.GoIdent, "{")
	for _, field := range bc.msg.Fields {
		if isPostAssignField(field) {
			continue
		}
		varName := bc.varPrefix + "_" + string(field.Desc.Name())
		emitFieldAssignment(g, field, varName, indent+"\t")
	}
	g.P(indent, "}")

	// Post-assign complex fields
	for _, field := range bc.msg.Fields {
		if !isPostAssignField(field) {
			continue
		}
		varName := bc.varPrefix + "_" + string(field.Desc.Name())
		emitPostAssignFieldCode(g, field, varName, tmpVar, indent)
	}

	g.P(indent, "req.", bc.goFieldName, " = ", tmpVar)
}

func findMethod(svc *protogen.Service, goName string) *protogen.Method {
	for _, m := range svc.Methods {
		if m.GoName == goName {
			return m
		}
	}
	return nil
}

// resolveBindIntoCtxs walks the tree and resolves bind_into contexts
// from the service's method descriptors.
func resolveBindIntoCtxs(tree *treeNode, svc *protogen.Service) []bindIntoCtx {
	var ctxs []bindIntoCtx
	collectBindInto(tree, svc, &ctxs)
	return ctxs
}

func collectBindInto(node *treeNode, svc *protogen.Service, ctxs *[]bindIntoCtx) {
	if node.BindInto != "" {
		if bc := resolveOneBindInto(node.BindInto, svc); bc != nil {
			*ctxs = append(*ctxs, *bc)
		}
	}
	for _, child := range node.Children {
		collectBindInto(child, svc, ctxs)
	}
}

func resolveOneBindInto(fieldName string, svc *protogen.Service) *bindIntoCtx {
	for _, m := range svc.Methods {
		for _, f := range m.Input.Fields {
			if string(f.Desc.Name()) == fieldName && f.Message != nil {
				return &bindIntoCtx{
					fieldName:   fieldName,
					goFieldName: f.GoName,
					msg:         f.Message,
					varPrefix:   "bind_" + fieldName,
				}
			}
		}
	}
	return nil
}

func findBindCtx(ctxs []bindIntoCtx, fieldName string) *bindIntoCtx {
	for i := range ctxs {
		if ctxs[i].fieldName == fieldName {
			return &ctxs[i]
		}
	}
	return nil
}

// emitBindIntoVars emits variable declarations for a bind_into
// message's fields at function scope.
func emitBindIntoVars(g *protogen.GeneratedFile, bc bindIntoCtx) {
	for _, field := range bc.msg.Fields {
		varName := bc.varPrefix + "_" + string(field.Desc.Name())
		emitVarDecl(g, field, varName, "\t")
	}
}

func emitVarDecl(g *protogen.GeneratedFile, field *protogen.Field, varName, indent string) {
	// Map fields → string for JSON
	if field.Desc.IsMap() {
		g.P(indent, "var ", varName, " string")
		return
	}

	// Repeated fields
	if field.Desc.IsList() {
		switch field.Desc.Kind() {
		case protoreflect.MessageKind, protoreflect.BytesKind, protoreflect.GroupKind:
			g.P(indent, "var ", varName, " string")
		case protoreflect.EnumKind:
			g.P(indent, "var ", varName, " []int32")
		case protoreflect.StringKind:
			g.P(indent, "var ", varName, " []string")
		case protoreflect.BoolKind,
			protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
			protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
			protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
			protoreflect.Uint64Kind, protoreflect.Fixed64Kind,
			protoreflect.FloatKind, protoreflect.DoubleKind:
			ri := resolveRepeatedFieldType(field)
			if ri != nil {
				g.P(indent, "var ", varName, " ", ri.goType)
			}
		}
		return
	}

	// Singular enum
	if field.Desc.Kind() == protoreflect.EnumKind {
		g.P(indent, "var ", varName, " int32")
		return
	}

	// Singular scalar/message/bytes
	ti := resolveFieldType(field)
	if ti != nil {
		g.P(indent, "var ", varName, " ", ti.goType)
	}
}

// emitBindIntoFlagRegistration registers flags on the group's FlagSet
// for the bind_into message's fields.
func emitBindIntoFlagRegistration(g *protogen.GeneratedFile, bc *bindIntoCtx, indent string) {
	for _, field := range bc.msg.Fields {
		varName := bc.varPrefix + "_" + string(field.Desc.Name())
		flagName := fieldToFlagName(string(field.Desc.Name()))
		emitBindIntoFlagReg(g, field, varName, flagName, indent)
	}
}

func emitBindIntoFlagReg(
	g *protogen.GeneratedFile,
	field *protogen.Field,
	varName, flagName, indent string,
) {
	usage := fieldUsage(field)
	qName := fmt.Sprintf("%q", flagName)
	qUsage := fmt.Sprintf("%q", usage)

	// Map fields → string flag
	if field.Desc.IsMap() {
		g.P(indent, "fs.StringVar(&", varName, ", ", qName, ", ", `""`, ", ", qUsage, ")")
		return
	}

	// Repeated fields
	if field.Desc.IsList() {
		switch field.Desc.Kind() {
		case protoreflect.MessageKind, protoreflect.BytesKind, protoreflect.GroupKind:
			g.P(indent, "fs.StringVar(&", varName, ", ", qName, ", ", `""`, ", ", qUsage, ")")
		case protoreflect.EnumKind:
			typeName := string(field.Enum.Desc.Name())
			g.P(
				indent, "fs.Var(",
				cliPkg.Ident("NewEnumSliceValue"),
				"(&", varName, ", []", cliPkg.Ident("EnumMapping"), "{",
			)
			emitEnumMappingEntries(g, field.Enum, indent)
			g.P(indent, "}, ", fmt.Sprintf("%q", typeName), "), ", qName, ", ", qUsage, ")")
		case protoreflect.StringKind:
			emitFsVar(g, indent, "NewStringSliceValue", varName, flagName, usage)
		case protoreflect.BoolKind,
			protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
			protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
			protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
			protoreflect.Uint64Kind, protoreflect.Fixed64Kind,
			protoreflect.FloatKind, protoreflect.DoubleKind:
			ri := resolveRepeatedFieldType(field)
			if ri != nil {
				emitFsVar(g, indent, ri.constructor, varName, flagName, usage)
			}
		}
		return
	}

	// Singular enum
	if field.Desc.Kind() == protoreflect.EnumKind {
		emitEnumFlagReg(g, field, varName, flagName, indent, usage)
		return
	}

	// Singular scalar/message/bytes
	ti := resolveFieldType(field)
	if ti != nil {
		emitFlagReg(g, indent, ti, varName, flagName, usage)
	}
}

// emitEnumFlagReg registers an enum flag using the given variable name
// (for bind_into contexts where the var prefix differs from "flag_").
func emitEnumFlagReg(
	g *protogen.GeneratedFile,
	field *protogen.Field,
	varName, flagName, indent, usage string,
) {
	typeName := string(field.Enum.Desc.Name())

	g.P(indent, "fs.Var(", cliPkg.Ident("NewEnumValue"), "(&", varName, ", []", cliPkg.Ident("EnumMapping"), "{")
	emitEnumMappingEntries(g, field.Enum, indent)
	g.P(
		indent,
		"}, ",
		fmt.Sprintf("%q", typeName),
		"), ",
		fmt.Sprintf("%q", flagName),
		", ",
		fmt.Sprintf("%q", usage),
		")",
	)
}

// hasBindIntoComplexFields returns true if the bind_into message contains
// any fields requiring post-assignment (JSON string, bytes, map).
func hasBindIntoComplexFields(bc *bindIntoCtx) bool {
	return slices.ContainsFunc(bc.msg.Fields, isPostAssignField)
}

// emitBindIntoFieldConstruction emits the bind_into message field
// construction in the request struct literal.
// If the bind_into message has complex fields, it emits only the field name
// as a placeholder (the actual construction is in emitRequestPostAssign).
func emitBindIntoFieldConstruction(g *protogen.GeneratedFile, bc *bindIntoCtx, indent string) {
	if hasBindIntoComplexFields(bc) {
		// Complex bind_into: build separately and assign post struct literal.
		// Emit nothing in the struct literal — it will be assigned via post-assign.
		return
	}

	// Simple bind_into: inline struct literal
	g.P(indent, bc.goFieldName, ": &", bc.msg.GoIdent, "{")
	for _, field := range bc.msg.Fields {
		varName := bc.varPrefix + "_" + string(field.Desc.Name())
		emitFieldAssignment(g, field, varName, indent+"\t")
	}
	g.P(indent, "},")
}

// emitSchemaNode generates the "schema" subcommand leaf that prints
// procedure type information as JSON.
func emitSchemaDataDecls(
	g *protogen.GeneratedFile,
	entries []schemaEntry,
	names schemaDataVarNames,
) {
	g.P("var ", names.byCommand, " = map[string]", cliPkg.Ident("CommandInfo"), "{")
	for _, entry := range entries {
		g.P("\t", fmt.Sprintf("%q", entry.commandPath), ": ")
		emitSchemaInfoLiteral(g, entry, "\t")
	}
	g.P("}")
	g.P()

	g.P("var ", names.byProcedure, " = map[string]", cliPkg.Ident("CommandInfo"), "{")
	for _, entry := range entries {
		g.P("\t", fmt.Sprintf("%q", entry.procedure), ": ")
		emitSchemaInfoLiteral(g, entry, "\t")
	}
	g.P("}")
	g.P()

	g.P("var ", names.all, " = []", cliPkg.Ident("CommandInfo"), "{")
	for _, entry := range entries {
		emitSchemaInfoLiteral(g, entry, "\t")
	}
	g.P("}")
	g.P()
}

func emitSchemaInfoLiteral(g *protogen.GeneratedFile, entry schemaEntry, indent string) {
	g.P(indent, "{")
	g.P(indent, "\tCommand: ", fmt.Sprintf("%q", entry.commandPath), ",")
	if entry.summary != "" {
		g.P(indent, "\tSummary: ", fmt.Sprintf("%q", entry.summary), ",")
	}
	g.P(indent, "\tProcedure: ", fmt.Sprintf("%q", entry.procedure), ",")
	emitSchemaFields(g, entry.input, "Flags", indent+"\t")
	emitSchemaFields(g, entry.output, "Output", indent+"\t")
	if entry.streaming {
		g.P(indent, "\tStreaming: true,")
	}
	g.P(indent, "},")
}

func emitSchemaNode(
	g *protogen.GeneratedFile,
	rootVar string,
	names schemaDataVarNames,
) {
	g.P("\t", rootVar, `.Children["schema"] = &`, cliPkg.Ident("Node"), "{")
	g.P("\t\tSegment: \"schema\",")
	g.P("\t\tSummary: \"Show procedure schemas\",")
	g.P(
		"\t\tRun: func(_ ",
		contextPkg.Ident("Context"),
		", args []string, stdout ",
		ioPkg.Ident("Writer"),
		") error {",
	)

	emitSchemaLookup(g, names)

	g.P("\t\t},")
	g.P("\t}")
}

// emitSchemaLookup emits the lookup/output logic for the schema command.
func emitSchemaLookup(g *protogen.GeneratedFile, names schemaDataVarNames) {
	g.P("\t\t\tif len(args) > 0 {")
	g.P("\t\t\t\tkey := ", stringsPkg.Ident("Join"), "(args, \" \")")
	g.P("\t\t\t\tinfo, ok := ", names.byCommand, "[key]")
	g.P("\t\t\t\tif !ok {")
	g.P("\t\t\t\t\tinfo, ok = ", names.byProcedure, "[key]")
	g.P("\t\t\t\t}")
	g.P("\t\t\t\tif !ok {")
	g.P(
		"\t\t\t\t\treturn ",
		fmtPkg.Ident("Errorf"),
		`("unknown command %q", key)`,
	)
	g.P("\t\t\t\t}")
	g.P(
		"\t\t\t\tout, err := ",
		encodingJSONPkg.Ident("MarshalIndent"),
		`(info, "", "  ")`,
	)
	g.P("\t\t\t\tif err != nil {")
	g.P("\t\t\t\t\treturn err")
	g.P("\t\t\t\t}")
	g.P("\t\t\t\t", fmtPkg.Ident("Fprintln"), "(stdout, string(out))")
	g.P("\t\t\t\treturn nil")
	g.P("\t\t\t}")

	g.P(
		"\t\t\tout, err := ",
		encodingJSONPkg.Ident("MarshalIndent"),
		"(",
		names.all,
		`, "", "  ")`,
	)
	g.P("\t\t\tif err != nil {")
	g.P("\t\t\t\treturn err")
	g.P("\t\t\t}")
	g.P("\t\t\t", fmtPkg.Ident("Fprintln"), "(stdout, string(out))")
	g.P("\t\t\treturn nil")
}

// emitSchemaFields emits a []SchemaField literal for the given message type.
func emitSchemaFields(
	g *protogen.GeneratedFile,
	msg *protogen.Message,
	fieldName, indent string,
) {
	g.P(indent, fieldName, ": []", cliPkg.Ident("SchemaField"), "{")
	for _, field := range msg.Fields {
		emitSchemaFieldEntry(g, field, indent)
	}
	g.P(indent, "},")
}

func emitSchemaFieldEntry(
	g *protogen.GeneratedFile,
	field *protogen.Field,
	indent string,
) {
	g.P(indent, "\t{")
	g.P(indent, "\t\tName: ", fmt.Sprintf("%q", string(field.Desc.Name())), ",")
	g.P(indent, "\t\tType: ", fmt.Sprintf("%q", schemaFieldType(field)), ",")
	if desc := getFieldDescription(field); desc != "" {
		g.P(indent, "\t\tDescription: ", fmt.Sprintf("%q", desc), ",")
	}
	if field.Desc.IsList() {
		g.P(indent, "\t\tRepeated: true,")
	}
	if field.Desc.IsMap() {
		g.P(indent, "\t\tMap: true,")
	}
	if field.Desc.Kind() == protoreflect.EnumKind {
		values := enumCLIValuesForSchema(field.Enum)
		if len(values) > 0 {
			g.P(indent, "\t\tEnumValues: []string{")
			for _, v := range values {
				g.P(indent, "\t\t\t", fmt.Sprintf("%q", v), ",")
			}
			g.P(indent, "\t\t},")
		}
	}
	g.P(indent, "\t},")
}

// schemaFieldType returns the type string for a schema field.
func schemaFieldType(field *protogen.Field) string {
	if field.Desc.IsMap() {
		keyType := schemaKindString(field.Desc.MapKey().Kind())
		valType := schemaKindString(field.Desc.MapValue().Kind())
		return "map<" + keyType + "," + valType + ">"
	}
	return schemaKindString(field.Desc.Kind())
}

// schemaKindString maps a protoreflect.Kind to a string for schema display.
func schemaKindString(kind protoreflect.Kind) string {
	switch kind {
	case protoreflect.StringKind:
		return "string"
	case protoreflect.BoolKind:
		return "bool"
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return "int32"
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return "int64"
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return "uint32"
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return "uint64"
	case protoreflect.FloatKind:
		return "float"
	case protoreflect.DoubleKind:
		return "double"
	case protoreflect.EnumKind:
		return "enum"
	case protoreflect.MessageKind:
		return "message"
	case protoreflect.BytesKind:
		return "bytes"
	case protoreflect.GroupKind:
		return "group"
	default:
		return "unknown"
	}
}

// enumCLIValuesForSchema returns the CLI value names for an enum type,
// excluding the UNSPECIFIED (0) value.
func enumCLIValuesForSchema(enumType *protogen.Enum) []string {
	mappings, err := enumCLIValues(string(enumType.Desc.Name()), extractEnumValueInfos(enumType))
	if err != nil {
		return nil
	}
	result := make([]string, 0, len(mappings))
	for _, m := range mappings {
		result = append(result, m.CLIValue)
	}
	return result
}
