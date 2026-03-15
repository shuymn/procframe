package codegen

import (
	"google.golang.org/protobuf/compiler/protogen"
)

// generateHandler writes the handler interface for a service,
// producing one method per RPC.
func generateHandler(g *protogen.GeneratedFile, svc *protogen.Service) {
	g.P("// ", svc.GoName, "Handler is the handler interface for ", svc.GoName, ".")
	g.P("type ", svc.GoName, "Handler interface {")

	for _, m := range svc.Methods {
		if m.Desc.IsStreamingServer() {
			g.P("\t", m.GoName, "(")
			g.P("\t\t", contextPkg.Ident("Context"), ",")
			g.P("\t\t*", procframePkg.Ident("Request"), "[", m.Input.GoIdent, "],")
			g.P("\t\t", procframePkg.Ident("ServerStream"), "[", m.Output.GoIdent, "],")
			g.P("\t) error")
		} else {
			g.P("\t", m.GoName, "(")
			g.P("\t\t", contextPkg.Ident("Context"), ",")
			g.P("\t\t*", procframePkg.Ident("Request"), "[", m.Input.GoIdent, "],")
			g.P("\t) (*", procframePkg.Ident("Response"), "[", m.Output.GoIdent, "], error)")
		}
	}

	g.P("}")
}
