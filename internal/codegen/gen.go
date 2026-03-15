// Package gen implements code generation for protoc-gen-procframe-go.
package codegen

import (
	"google.golang.org/protobuf/compiler/protogen"
)

// Generate processes a single proto file and emits handler interface
// and CLI runner files for each service that has CLI-exposed methods.
func Generate(plugin *protogen.Plugin, file *protogen.File) error {
	if len(file.Services) == 0 {
		return nil
	}

	// Extract service info from proto descriptors
	services := make([]serviceInfo, 0, len(file.Services))
	for _, svc := range file.Services {
		services = append(services, extractServiceInfo(svc))
	}

	// Validate
	if err := validateDuplicatePaths(services); err != nil {
		return err
	}
	if err := validateEnumCollisions(plugin); err != nil {
		return err
	}
	if err := validateBindInto(services, plugin); err != nil {
		return err
	}

	// Generate per-service files
	for i, svc := range file.Services {
		svcInfo := &services[i]

		tree := buildTree([]serviceInfo{*svcInfo})

		generateHandler(plugin, file, svc)

		if hasCliMethods(svcInfo) {
			generateCLI(plugin, file, svc, svcInfo, tree)
		}
	}

	return nil
}

func hasCliMethods(svc *serviceInfo) bool {
	for _, m := range svc.Methods {
		if m.CLI {
			return true
		}
	}
	return false
}
