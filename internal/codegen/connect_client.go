package codegen

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"
)

var connectRPCPkg = protogen.GoImportPath("connectrpc.com/connect")

// unexport lowercases the first letter of a Go name.
func unexport(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]+'a'-'A') + s[1:]
}

// generateConnectClient writes the Connect client interface, implementation,
// and constructor for a service.
func generateConnectClient(
	g *protogen.GeneratedFile,
	svc *protogen.Service,
	svcInfo *serviceInfo,
) {
	ifaceName := svc.GoName + "ConnectClient"
	implName := unexport(svc.GoName) + "ConnectClient"
	funcName := "New" + svc.GoName + "ConnectClient"

	// Collect connect-enabled methods.
	type connectMethod struct {
		method *protogen.Method
		info   *methodInfo
	}
	var methods []connectMethod
	for i, m := range svc.Methods {
		mi := &svcInfo.Methods[i]
		if mi.Connect {
			methods = append(methods, connectMethod{method: m, info: mi})
		}
	}

	// --- Interface ---
	g.P("// ", ifaceName, " is the client interface for ", svc.GoName, " over Connect.")
	g.P("type ", ifaceName, " interface {")
	for _, cm := range methods {
		emitClientInterfaceMethod(g, cm.method, cm.info)
	}
	g.P("}")
	g.P()

	// --- Implementation struct ---
	g.P("type ", implName, " struct {")
	for _, cm := range methods {
		fieldName := unexport(cm.method.GoName)
		g.P("\t", fieldName, " *", connectRPCPkg.Ident("Client"), "[",
			cm.method.Input.GoIdent, ", ", cm.method.Output.GoIdent, "]")
	}
	g.P("}")
	g.P()

	// --- Constructor ---
	g.P("// ", funcName, " constructs a Connect client for ", svc.GoName, ".")
	g.P("func ", funcName, "(httpClient ", connectRPCPkg.Ident("HTTPClient"),
		", baseURL string, opts ...", connectRPCPkg.Ident("ClientOption"),
		") ", ifaceName, " {")
	g.P("\treturn &", implName, "{")
	for _, cm := range methods {
		fieldName := unexport(cm.method.GoName)
		procedure := fmt.Sprintf("%q", cm.info.FullName)
		g.P("\t\t", fieldName, ": ", connectRPCPkg.Ident("NewClient"),
			"[", cm.method.Input.GoIdent, ", ", cm.method.Output.GoIdent, "](",
			"httpClient, baseURL+", procedure, ", opts...),")
	}
	g.P("\t}")
	g.P("}")
	g.P()

	// --- Method implementations ---
	for _, cm := range methods {
		emitClientMethodImpl(g, cm.method, cm.info, implName)
	}
}

func emitClientInterfaceMethod(g *protogen.GeneratedFile, m *protogen.Method, mi *methodInfo) {
	switch mi.Shape {
	case shapeClientStream:
		g.P("\t", m.GoName, "(ctx ", contextPkg.Ident("Context"),
			") *", connectRPCPkg.Ident("ClientStreamForClient"),
			"[", m.Input.GoIdent, ", ", m.Output.GoIdent, "]")
	case shapeServerStream:
		g.P("\t", m.GoName, "(ctx ", contextPkg.Ident("Context"),
			", req *", connectRPCPkg.Ident("Request"),
			"[", m.Input.GoIdent, "]) (*",
			connectRPCPkg.Ident("ServerStreamForClient"),
			"[", m.Output.GoIdent, "], error)")
	case shapeBidi:
		g.P("\t", m.GoName, "(ctx ", contextPkg.Ident("Context"),
			") *", connectRPCPkg.Ident("BidiStreamForClient"),
			"[", m.Input.GoIdent, ", ", m.Output.GoIdent, "]")
	default: // unary
		g.P("\t", m.GoName, "(ctx ", contextPkg.Ident("Context"),
			", req *", connectRPCPkg.Ident("Request"),
			"[", m.Input.GoIdent, "]) (*",
			connectRPCPkg.Ident("Response"),
			"[", m.Output.GoIdent, "], error)")
	}
}

func emitClientMethodImpl(g *protogen.GeneratedFile, m *protogen.Method, mi *methodInfo, implName string) {
	fieldName := unexport(m.GoName)
	receiver := "c"

	switch mi.Shape {
	case shapeClientStream:
		g.P("func (", receiver, " *", implName, ") ", m.GoName,
			"(ctx ", contextPkg.Ident("Context"),
			") *", connectRPCPkg.Ident("ClientStreamForClient"),
			"[", m.Input.GoIdent, ", ", m.Output.GoIdent, "] {")
		g.P("\treturn ", receiver, ".", fieldName, ".CallClientStream(ctx)")
		g.P("}")
	case shapeServerStream:
		g.P("func (", receiver, " *", implName, ") ", m.GoName,
			"(ctx ", contextPkg.Ident("Context"),
			", req *", connectRPCPkg.Ident("Request"),
			"[", m.Input.GoIdent, "]) (*",
			connectRPCPkg.Ident("ServerStreamForClient"),
			"[", m.Output.GoIdent, "], error) {")
		g.P("\treturn ", receiver, ".", fieldName, ".CallServerStream(ctx, req)")
		g.P("}")
	case shapeBidi:
		g.P("func (", receiver, " *", implName, ") ", m.GoName,
			"(ctx ", contextPkg.Ident("Context"),
			") *", connectRPCPkg.Ident("BidiStreamForClient"),
			"[", m.Input.GoIdent, ", ", m.Output.GoIdent, "] {")
		g.P("\treturn ", receiver, ".", fieldName, ".CallBidiStream(ctx)")
		g.P("}")
	default: // unary
		g.P("func (", receiver, " *", implName, ") ", m.GoName,
			"(ctx ", contextPkg.Ident("Context"),
			", req *", connectRPCPkg.Ident("Request"),
			"[", m.Input.GoIdent, "]) (*",
			connectRPCPkg.Ident("Response"),
			"[", m.Output.GoIdent, "], error) {")
		g.P("\treturn ", receiver, ".", fieldName, ".CallUnary(ctx, req)")
		g.P("}")
	}
	g.P()
}
