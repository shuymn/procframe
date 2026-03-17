package codegen

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"
)

// generateWSClient writes the WS client interface, implementation,
// and constructor for a service.
func generateWSClient(
	g *protogen.GeneratedFile,
	svc *protogen.Service,
	svcInfo *serviceInfo,
) {
	ifaceName := svc.GoName + "WSClient"
	implName := unexport(svc.GoName) + "WSClient"
	funcName := "New" + svc.GoName + "WSClient"

	// Collect ws-enabled methods.
	type wsMethod struct {
		method *protogen.Method
		info   *methodInfo
	}
	var methods []wsMethod
	for i, m := range svc.Methods {
		mi := &svcInfo.Methods[i]
		if mi.Ws {
			methods = append(methods, wsMethod{method: m, info: mi})
		}
	}

	// --- Interface ---
	g.P("// ", ifaceName, " is the client interface for ", svc.GoName, " over WebSocket.")
	g.P("type ", ifaceName, " interface {")
	for _, wm := range methods {
		emitWSClientInterfaceMethod(g, wm.method, wm.info)
	}
	g.P("}")
	g.P()

	// --- Implementation struct ---
	g.P("type ", implName, " struct {")
	g.P("\tconn *", wsPkg.Ident("Conn"))
	g.P("}")
	g.P()

	// --- Constructor ---
	g.P("// ", funcName, " constructs a WebSocket client for ", svc.GoName, ".")
	g.P("func ", funcName, "(conn *", wsPkg.Ident("Conn"), ") ", ifaceName, " {")
	g.P("\treturn &", implName, "{conn: conn}")
	g.P("}")
	g.P()

	// --- Method implementations ---
	for _, wm := range methods {
		emitWSClientMethodImpl(g, wm.method, wm.info, implName)
	}
}

func emitWSClientInterfaceMethod(g *protogen.GeneratedFile, m *protogen.Method, mi *methodInfo) {
	switch mi.Shape {
	case shapeUnary:
		g.P("\t", m.GoName, "(ctx ", contextPkg.Ident("Context"),
			", req *", m.Input.GoIdent,
			") (*", m.Output.GoIdent, ", error)")
	case shapeServerStream:
		g.P("\t", m.GoName, "(ctx ", contextPkg.Ident("Context"),
			", req *", m.Input.GoIdent,
			") (", wsPkg.Ident("ServerStream"), "[", m.Output.GoIdent, "], error)")
	case shapeClientStream:
		g.P("\t", m.GoName, "(ctx ", contextPkg.Ident("Context"),
			") (", wsPkg.Ident("ClientStream"), "[", m.Input.GoIdent, ", ", m.Output.GoIdent, "], error)")
	case shapeBidi:
		g.P("\t", m.GoName, "(ctx ", contextPkg.Ident("Context"),
			") (", wsPkg.Ident("BidiStream"), "[", m.Input.GoIdent, ", ", m.Output.GoIdent, "], error)")
	}
}

func emitWSClientMethodImpl(g *protogen.GeneratedFile, m *protogen.Method, mi *methodInfo, implName string) {
	receiver := "c"
	procedure := fmt.Sprintf("%q", mi.FullName)

	switch mi.Shape {
	case shapeUnary:
		g.P("func (", receiver, " *", implName, ") ", m.GoName,
			"(ctx ", contextPkg.Ident("Context"),
			", req *", m.Input.GoIdent,
			") (*", m.Output.GoIdent, ", error) {")
		g.P("\treturn ", wsPkg.Ident("CallUnary"),
			"[", m.Input.GoIdent, ", ", m.Output.GoIdent, "]",
			"(ctx, ", receiver, ".conn, ", procedure, ", req)")
		g.P("}")
	case shapeServerStream:
		g.P("func (", receiver, " *", implName, ") ", m.GoName,
			"(ctx ", contextPkg.Ident("Context"),
			", req *", m.Input.GoIdent,
			") (", wsPkg.Ident("ServerStream"), "[", m.Output.GoIdent, "], error) {")
		g.P("\treturn ", wsPkg.Ident("CallServerStream"),
			"[", m.Input.GoIdent, ", ", m.Output.GoIdent, "]",
			"(ctx, ", receiver, ".conn, ", procedure, ", req)")
		g.P("}")
	case shapeClientStream:
		g.P("func (", receiver, " *", implName, ") ", m.GoName,
			"(ctx ", contextPkg.Ident("Context"),
			") (", wsPkg.Ident("ClientStream"), "[", m.Input.GoIdent, ", ", m.Output.GoIdent, "], error) {")
		g.P("\treturn ", wsPkg.Ident("CallClientStream"),
			"[", m.Input.GoIdent, ", ", m.Output.GoIdent, "]",
			"(ctx, ", receiver, ".conn, ", procedure, ")")
		g.P("}")
	case shapeBidi:
		g.P("func (", receiver, " *", implName, ") ", m.GoName,
			"(ctx ", contextPkg.Ident("Context"),
			") (", wsPkg.Ident("BidiStream"), "[", m.Input.GoIdent, ", ", m.Output.GoIdent, "], error) {")
		g.P("\treturn ", wsPkg.Ident("CallBidi"),
			"[", m.Input.GoIdent, ", ", m.Output.GoIdent, "]",
			"(ctx, ", receiver, ".conn, ", procedure, ")")
		g.P("}")
	}
	g.P()
}
