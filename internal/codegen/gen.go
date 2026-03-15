// Package gen implements code generation for protoc-gen-procframe-go.
package codegen

import (
	"google.golang.org/protobuf/compiler/protogen"
)

// Generate processes a single proto file and emits handler interface
// and CLI runner files for each service that has CLI-exposed methods.
func Generate(plugin *protogen.Plugin, file *protogen.File, params *Params) error {
	services, cfgInfo, err := extractGenerationInputs(file, params)
	if err != nil {
		return err
	}
	if len(services) == 0 && cfgInfo == nil {
		return nil
	}

	if err := validateGenerationInputs(plugin, services, cfgInfo, params); err != nil {
		return err
	}

	if cfgInfo != nil {
		if err := generateConfig(plugin, file, cfgInfo); err != nil {
			return err
		}
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

func extractGenerationInputs(file *protogen.File, params *Params) ([]serviceInfo, *configInfo, error) {
	services := make([]serviceInfo, 0, len(file.Services))
	for _, svc := range file.Services {
		services = append(services, extractServiceInfo(svc))
	}

	cfgInfo, err := extractConfigInfo(file, params)
	if err != nil {
		return nil, nil, err
	}

	return services, cfgInfo, nil
}

func validateGenerationInputs(
	plugin *protogen.Plugin,
	services []serviceInfo,
	cfgInfo *configInfo,
	params *Params,
) error {
	if err := validateDuplicatePaths(services); err != nil {
		return err
	}
	if err := validateEnumCollisions(plugin); err != nil {
		return err
	}
	if err := validateBindInto(services, plugin); err != nil {
		return err
	}
	if err := validateConfigInfo(cfgInfo, params); err != nil {
		return err
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
