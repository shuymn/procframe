package codegen

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var sortPkg = protogen.GoImportPath("sort")

var (
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

	// Collect all nodes bottom-up and assign to variables
	var decls []nodeDecl
	collectDecls(tree, nil, svc, svcInfo, &decls)

	// Resolve bind_into contexts
	bindCtxs := resolveBindIntoCtxs(tree, svc)

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
	emitSchemaNode(g, svc, svcInfo, rootVar)

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
	g.P("\t}")
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
		"\t\t\t\t\treturn &",
		procframePkg.Ident("Error"),
		`{Code: `,
		procframePkg.Ident("CodeInvalidArgument"),
		`, Message: "--json cannot be combined with flags"}`,
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
	emitFlagVars(g, method.Input, "\t\t\t\t")
	g.P("\t\t\t\tif err := fs.Parse(args); err != nil {")
	g.P("\t\t\t\t\treturn err")
	g.P("\t\t\t\t}")
	g.P("\t\t\t\treq = &", method.Input.GoIdent, "{")
	emitRequestFields(g, method.Input, "\t\t\t\t\t", bindCtxs)
	g.P("\t\t\t\t}")
	g.P("\t\t\t}")
}

// emitHandlerCall emits the unary handler invocation and JSON output code
// within a leaf command's Run function.
func emitHandlerCall(g *protogen.GeneratedFile, method *protogen.Method, leaf *leafInfo) {
	g.P(
		"\t\t\tresp, err := h.",
		method.GoName,
		"(ctx, &",
		procframePkg.Ident("Request"),
		"[",
		method.Input.GoIdent,
		"]{",
	)
	g.P("\t\t\t\tMsg:  req,")
	g.P("\t\t\t\tMeta: ", procframePkg.Ident("Meta"), "{Procedure: ", fmt.Sprintf("%q", leaf.FullName), "},")
	g.P("\t\t\t})")
	g.P("\t\t\tif err != nil {")
	g.P("\t\t\t\treturn err")
	g.P("\t\t\t}")
	g.P("\t\t\tif resp == nil || resp.Msg == nil {")
	g.P(
		"\t\t\t\treturn &",
		procframePkg.Ident("Error"),
		"{Code: ",
		procframePkg.Ident("CodeInternal"),
		`, Message: "handler returned nil response"}`,
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
		"\t\t\t\t\treturn &",
		procframePkg.Ident("Error"),
		"{Code: ",
		procframePkg.Ident("CodeInternal"),
		`, Message: "handler sent nil response"}`,
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
		"\t\t\treturn h.",
		method.GoName,
		"(ctx, &",
		procframePkg.Ident("Request"),
		"[",
		method.Input.GoIdent,
		"]{",
	)
	g.P("\t\t\t\tMsg:  req,")
	g.P("\t\t\t\tMeta: ", procframePkg.Ident("Meta"), "{Procedure: ", fmt.Sprintf("%q", leaf.FullName), "},")
	g.P("\t\t\t}, stream)")
}

func emitFlagVars(g *protogen.GeneratedFile, msg *protogen.Message, indent string) {
	for _, field := range msg.Fields {
		emitFlagVar(g, field, indent)
	}
}

func emitFlagVar(g *protogen.GeneratedFile, field *protogen.Field, indent string) {
	if field.Desc.IsList() && field.Desc.Kind() == protoreflect.StringKind {
		name := fieldToFlagName(string(field.Desc.Name()))
		varName := "flag_" + string(field.Desc.Name())
		g.P(indent, "var ", varName, " []string")
		emitFsVar(g, indent, "NewStringSliceValue", varName, name)
		return
	}

	if field.Desc.Kind() == protoreflect.EnumKind {
		emitEnumFlagVar(g, field, indent)
		return
	}

	ti := resolveFieldType(field)
	if ti == nil {
		return
	}

	name := fieldToFlagName(string(field.Desc.Name()))
	varName := "flag_" + string(field.Desc.Name())
	g.P(indent, "var ", varName, " ", ti.goType)
	emitFlagReg(g, indent, ti, varName, name)
}

func emitFsVar(g *protogen.GeneratedFile, indent, constructor, varName, flagName string) {
	g.P(indent, "fs.Var(", cliPkg.Ident(constructor), "(&", varName, "), ", fmt.Sprintf("%q", flagName), ", \"\")")
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
	case protoreflect.EnumKind, protoreflect.BytesKind, protoreflect.MessageKind, protoreflect.GroupKind:
		return nil
	default:
		return nil
	}
}

func emitFlagReg(g *protogen.GeneratedFile, indent string, ti *fieldTypeInfo, varName, flagName string) {
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
			", \"\")",
		)
	} else {
		emitFsVar(g, indent, ti.constructor, varName, flagName)
	}
}

func emitEnumFlagVar(g *protogen.GeneratedFile, field *protogen.Field, indent string) {
	name := fieldToFlagName(string(field.Desc.Name()))
	varName := "flag_" + string(field.Desc.Name())
	typeName := string(field.Enum.Desc.Name())

	g.P(indent, "var ", varName, " int32")
	g.P(indent, "fs.Var(", cliPkg.Ident("NewEnumValue"), "(&", varName, ", []", cliPkg.Ident("EnumMapping"), "{")
	emitEnumMappingEntries(g, field.Enum, indent)
	g.P(indent, "}, ", fmt.Sprintf("%q", typeName), "), ", fmt.Sprintf("%q", name), ", \"\")")
}

// emitEnumMappingEntries emits []cli.EnumMapping literal entries
// using enumCLIValues to ensure consistent transform with validation.
func emitEnumMappingEntries(g *protogen.GeneratedFile, enumType *protogen.Enum, indent string) {
	values := make([]*enumValueInfo, 0, len(enumType.Values))
	for _, v := range enumType.Values {
		values = append(values, &enumValueInfo{
			ProtoName: string(v.Desc.Name()),
			Number:    int32(v.Desc.Number()),
		})
	}
	// enumCLIValues error is impossible here: validateEnumCollisions
	// already ran.
	mappings, err := enumCLIValues(string(enumType.Desc.Name()), values)
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

		switch {
		case field.Desc.Kind() == protoreflect.MessageKind,
			field.Desc.Kind() == protoreflect.BytesKind,
			field.Desc.Kind() == protoreflect.GroupKind:
			continue

		case field.Desc.Kind() == protoreflect.EnumKind:
			g.P(indent, field.GoName, ": ", field.Enum.GoIdent, "(", varName, "),")

		default:
			g.P(indent, field.GoName, ": ", varName, ",")
		}
	}
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

// isBindIntoSupportedField returns true if the field can be exposed
// as a group flag. Unsupported: message, bytes, repeated fields.
func isBindIntoSupportedField(field *protogen.Field) bool {
	if field.Desc.IsList() {
		return false
	}
	kind := field.Desc.Kind()
	return kind != protoreflect.MessageKind &&
		kind != protoreflect.GroupKind &&
		kind != protoreflect.BytesKind
}

// emitBindIntoVars emits variable declarations for a bind_into
// message's fields at function scope.
func emitBindIntoVars(g *protogen.GeneratedFile, bc bindIntoCtx) {
	for _, field := range bc.msg.Fields {
		if !isBindIntoSupportedField(field) {
			continue
		}
		varName := bc.varPrefix + "_" + string(field.Desc.Name())
		emitVarDecl(g, field, varName, "\t")
	}
}

func emitVarDecl(g *protogen.GeneratedFile, field *protogen.Field, varName, indent string) {
	if field.Desc.Kind() == protoreflect.EnumKind {
		g.P(indent, "var ", varName, " int32")
		return
	}
	ti := resolveFieldType(field)
	if ti != nil {
		g.P(indent, "var ", varName, " ", ti.goType)
	}
}

// emitBindIntoFlagRegistration registers flags on the group's FlagSet
// for the bind_into message's fields.
func emitBindIntoFlagRegistration(g *protogen.GeneratedFile, bc *bindIntoCtx, indent string) {
	for _, field := range bc.msg.Fields {
		if !isBindIntoSupportedField(field) {
			continue
		}
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
	if field.Desc.Kind() == protoreflect.EnumKind {
		emitEnumFlagReg(g, field, varName, flagName, indent)
		return
	}
	ti := resolveFieldType(field)
	if ti != nil {
		emitFlagReg(g, indent, ti, varName, flagName)
	}
}

// emitEnumFlagReg registers an enum flag using the given variable name
// (for bind_into contexts where the var prefix differs from "flag_").
func emitEnumFlagReg(
	g *protogen.GeneratedFile,
	field *protogen.Field,
	varName, flagName, indent string,
) {
	typeName := string(field.Enum.Desc.Name())

	g.P(indent, "fs.Var(", cliPkg.Ident("NewEnumValue"), "(&", varName, ", []", cliPkg.Ident("EnumMapping"), "{")
	emitEnumMappingEntries(g, field.Enum, indent)
	g.P(indent, "}, ", fmt.Sprintf("%q", typeName), "), ", fmt.Sprintf("%q", flagName), ", \"\")")
}

// emitBindIntoFieldConstruction emits the bind_into message field
// construction in the request struct literal.
func emitBindIntoFieldConstruction(g *protogen.GeneratedFile, bc *bindIntoCtx, indent string) {
	g.P(indent, bc.goFieldName, ": &", bc.msg.GoIdent, "{")
	for _, field := range bc.msg.Fields {
		if !isBindIntoSupportedField(field) {
			continue
		}
		varName := bc.varPrefix + "_" + string(field.Desc.Name())
		if field.Desc.Kind() == protoreflect.EnumKind {
			g.P(indent, "\t", field.GoName, ": ", field.Enum.GoIdent, "(", varName, "),")
		} else {
			g.P(indent, "\t", field.GoName, ": ", varName, ",")
		}
	}
	g.P(indent, "},")
}

// emitSchemaNode generates the "schema" subcommand leaf that prints
// procedure type information as JSON.
func emitSchemaNode(
	g *protogen.GeneratedFile,
	svc *protogen.Service,
	svcInfo *serviceInfo,
	rootVar string,
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

	emitSchemaMap(g, svc, svcInfo)
	emitSchemaLookup(g)

	g.P("\t\t},")
	g.P("\t}")
}

// emitSchemaMap emits the static schema map within the schema command.
func emitSchemaMap(
	g *protogen.GeneratedFile,
	svc *protogen.Service,
	svcInfo *serviceInfo,
) {
	g.P("\t\t\tschemas := map[string]", cliPkg.Ident("CommandInfo"), "{")
	for _, mi := range svcInfo.Methods {
		if !mi.CLI {
			continue
		}
		method := findMethod(svc, mi.GoName)
		if method == nil {
			continue
		}
		cmdPath := strings.Join(slices.Concat(svcInfo.Path, mi.Path), " ")
		g.P("\t\t\t\t", fmt.Sprintf("%q", cmdPath), ": {")
		g.P("\t\t\t\t\tCommand: ", fmt.Sprintf("%q", cmdPath), ",")
		if mi.Summary != "" {
			g.P("\t\t\t\t\tSummary: ", fmt.Sprintf("%q", mi.Summary), ",")
		}
		g.P("\t\t\t\t\tProcedure: ", fmt.Sprintf("%q", mi.FullName), ",")
		emitSchemaFields(g, method.Input, "Flags", "\t\t\t\t\t")
		emitSchemaFields(g, method.Output, "Output", "\t\t\t\t\t")
		if mi.IsStreaming {
			g.P("\t\t\t\t\tStreaming: true,")
		}
		g.P("\t\t\t\t},")
	}
	g.P("\t\t\t}")
}

// emitSchemaLookup emits the lookup/output logic for the schema command.
func emitSchemaLookup(g *protogen.GeneratedFile) {
	g.P("\t\t\tif len(args) > 0 {")
	g.P("\t\t\t\tkey := ", stringsPkg.Ident("Join"), "(args, \" \")")
	g.P("\t\t\t\tinfo, ok := schemas[key]")
	g.P("\t\t\t\tif !ok {")
	// Fallback: linear scan by procedure name
	g.P("\t\t\t\t\tfor _, v := range schemas {")
	g.P("\t\t\t\t\t\tif v.Procedure == key {")
	g.P("\t\t\t\t\t\t\tinfo = v")
	g.P("\t\t\t\t\t\t\tok = true")
	g.P("\t\t\t\t\t\t\tbreak")
	g.P("\t\t\t\t\t\t}")
	g.P("\t\t\t\t\t}")
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

	g.P("\t\t\tall := make([]", cliPkg.Ident("CommandInfo"), ", 0, len(schemas))")
	g.P("\t\t\tfor _, info := range schemas {")
	g.P("\t\t\t\tall = append(all, info)")
	g.P("\t\t\t}")
	g.P("\t\t\t", sortPkg.Ident("Slice"), "(all, func(i, j int) bool {")
	g.P("\t\t\t\treturn all[i].Command < all[j].Command")
	g.P("\t\t\t})")
	g.P(
		"\t\t\tout, err := ",
		encodingJSONPkg.Ident("MarshalIndent"),
		`(all, "", "  ")`,
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
		g.P(indent, "\t{")
		g.P(indent, "\t\tName: ", fmt.Sprintf("%q", string(field.Desc.Name())), ",")
		g.P(indent, "\t\tType: ", fmt.Sprintf("%q", schemaFieldType(field)), ",")
		if field.Desc.IsList() {
			g.P(indent, "\t\tRepeated: true,")
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
	g.P(indent, "},")
}

// schemaFieldType returns the type string for a schema field.
func schemaFieldType(field *protogen.Field) string {
	switch field.Desc.Kind() {
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
	values := make([]*enumValueInfo, 0, len(enumType.Values))
	for _, v := range enumType.Values {
		values = append(values, &enumValueInfo{
			ProtoName: string(v.Desc.Name()),
			Number:    int32(v.Desc.Number()),
		})
	}
	mappings, err := enumCLIValues(string(enumType.Desc.Name()), values)
	if err != nil {
		return nil
	}
	result := make([]string, 0, len(mappings))
	for _, m := range mappings {
		result = append(result, m.CLIValue)
	}
	return result
}
