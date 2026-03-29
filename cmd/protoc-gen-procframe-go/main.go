// Command protoc-gen-procframe-go is a protoc plugin that generates
// handler interfaces and CLI runner code from service definitions
// annotated with procframe options.
package main

import (
	"fmt"
	"os"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/shuymn/procframe/internal/codegen"
	"github.com/shuymn/procframe/internal/version"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("protoc-gen-procframe-go " + version.Version)
		return
	}

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
