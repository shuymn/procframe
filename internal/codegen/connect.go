package codegen

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"
)

var (
	connectPkg = protogen.GoImportPath("github.com/shuymn/procframe/transport/connect")
	httpPkg    = protogen.GoImportPath("net/http")
)

// generateConnect writes the Connect handler constructor for a service.
func generateConnect(
	g *protogen.GeneratedFile,
	svc *protogen.Service,
	svcInfo *serviceInfo,
) {
	handlerType := svc.GoName + "Handler"
	funcName := "New" + svc.GoName + "ConnectHandler"

	servicePrefix := "/" + string(
		svc.Desc.ParentFile().Package(),
	) + "." + string(
		svc.Desc.Name(),
	) + "/"

	g.P("// ", funcName, " constructs a Connect protocol HTTP handler for ", svc.GoName, ".")
	g.P("// It returns the service path prefix and a handler that routes to each")
	g.P("// Connect-enabled RPC method.")
	g.P(
		"func ", funcName, "(h ", handlerType, ", opts ...", connectPkg.Ident("Option"),
		") (string, ", httpPkg.Ident("Handler"), ") {",
	)
	g.P("\tmux := ", httpPkg.Ident("NewServeMux"), "()")

	for i, m := range svc.Methods {
		mi := &svcInfo.Methods[i]
		if !mi.Connect {
			continue
		}
		emitConnectHandlerRegistration(g, m, mi)
	}

	g.P("\treturn ", fmt.Sprintf("%q", servicePrefix), ", mux")
	g.P("}")
	g.P()
}

func emitConnectHandlerRegistration(g *protogen.GeneratedFile, m *protogen.Method, mi *methodInfo) {
	procedure := fmt.Sprintf("%q", mi.FullName)
	var constructor string
	switch mi.Shape {
	case shapeServerStream:
		constructor = "NewServerStreamHandler"
	case shapeClientStream:
		constructor = "NewClientStreamHandler"
	case shapeBidi:
		constructor = "NewBidiStreamHandler"
	default:
		constructor = "NewUnaryHandler"
	}
	g.P("\tmux.Handle(", connectPkg.Ident(constructor), "(")
	g.P("\t\t", procedure, ",")
	g.P("\t\th.", m.GoName, ",")
	g.P("\t\topts...,")
	g.P("\t))")
}

// hasConnectMethods reports whether any method in the service has
// connect.enabled = true.
func hasConnectMethods(svc *serviceInfo) bool {
	for _, m := range svc.Methods {
		if m.Connect {
			return true
		}
	}
	return false
}
