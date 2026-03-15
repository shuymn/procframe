// Command protoc-gen-procframe-go is a protoc plugin that generates
// handler interfaces and CLI runner code from service definitions
// annotated with procframe options.
package main

import (
	"google.golang.org/protobuf/compiler/protogen"

	"github.com/shuymn/procframe/internal/codegen"
)

func main() {
	var params codegen.Params
	protogen.Options{
		ParamFunc: params.Set,
	}.Run(func(plugin *protogen.Plugin) error {
		for _, f := range plugin.Files {
			if !f.Generate {
				continue
			}
			if err := codegen.Generate(plugin, f, &params); err != nil {
				return err
			}
		}
		return nil
	})
}
