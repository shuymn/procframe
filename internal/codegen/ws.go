package codegen

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"
)

var wsPkg = protogen.GoImportPath("github.com/shuymn/procframe/transport/ws")

// generateWS writes the WS handler registration function for a service.
func generateWS(
	g *protogen.GeneratedFile,
	svc *protogen.Service,
	svcInfo *serviceInfo,
) {
	handlerType := svc.GoName + "Handler"
	funcName := "New" + svc.GoName + "WSHandler"

	g.P("// ", funcName, " registers WebSocket RPC handlers for ", svc.GoName, ".")
	g.P("// The handlers are registered on the given Server, which can be shared")
	g.P("// across multiple services.")
	g.P(
		"func ", funcName, "(s *", wsPkg.Ident("Server"),
		", h ", handlerType, ") {",
	)

	for i, m := range svc.Methods {
		mi := &svcInfo.Methods[i]
		if !mi.Ws {
			continue
		}
		procedure := fmt.Sprintf("%q", mi.FullName)
		switch mi.Shape {
		case shapeServerStream:
			g.P("\t", wsPkg.Ident("HandleServerStream"), "(s, ", procedure, ", h.", m.GoName, ")")
		case shapeClientStream:
			g.P("\t", wsPkg.Ident("HandleClientStream"), "(s, ", procedure, ", h.", m.GoName, ")")
		case shapeBidi:
			g.P("\t", wsPkg.Ident("HandleBidi"), "(s, ", procedure, ", h.", m.GoName, ")")
		default:
			g.P("\t", wsPkg.Ident("HandleUnary"), "(s, ", procedure, ", h.", m.GoName, ")")
		}
	}

	g.P("}")
	g.P()
}

// hasWsMethods reports whether any method in the service has
// ws.enabled = true.
func hasWsMethods(svc *serviceInfo) bool {
	for _, m := range svc.Methods {
		if m.Ws {
			return true
		}
	}
	return false
}
