package codegen

import (
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"

	procframeoptionsv1 "github.com/shuymn/procframe/gen/procframe/options/v1"
)

// extractServiceInfo reads proto options from a service descriptor and
// returns the intermediate representation used by tree building and
// code generation.
func extractServiceInfo(svc *protogen.Service) serviceInfo {
	info := serviceInfo{
		GoName: svc.GoName,
	}

	applyServiceOptions(&info, svc)

	for _, m := range svc.Methods {
		info.Methods = append(info.Methods, extractMethodInfo(m, svc))
	}

	return info
}

// applyServiceOptions reads cli_group options from the service descriptor
// and applies them to the serviceInfo.
func applyServiceOptions(info *serviceInfo, svc *protogen.Service) {
	opts := svc.Desc.Options()
	if opts == nil {
		return
	}
	ext := proto.GetExtension(opts, procframeoptionsv1.E_CliGroup)
	group, ok := ext.(*procframeoptionsv1.CliGroupOptions)
	if !ok || group == nil {
		return
	}
	if group.Path != nil {
		info.Path = group.Path.Segments
	}
	if group.BindInto != nil {
		info.BindInto = *group.BindInto
	}
	if group.Summary != nil {
		info.Summary = *group.Summary
	}
	if group.Hidden != nil {
		info.Hidden = *group.Hidden
	}
}

func extractMethodInfo(m *protogen.Method, svc *protogen.Service) methodInfo {
	info := methodInfo{
		GoName:       m.GoName,
		InputGoName:  m.Input.GoIdent.GoName,
		OutputGoName: m.Output.GoIdent.GoName,
		FullName: "/" + string(
			svc.Desc.ParentFile().Package(),
		) + "." + string(
			svc.Desc.Name(),
		) + "/" + string(
			m.Desc.Name(),
		),
		CLI:         true, // default
		IsStreaming: m.Desc.IsStreamingServer(),
	}

	applyMethodOptions(&info, m)

	return info
}

// applyMethodOptions reads proc options from the method descriptor
// and applies them to the methodInfo.
func applyMethodOptions(info *methodInfo, m *protogen.Method) {
	opts := m.Desc.Options()
	if opts == nil {
		return
	}
	ext := proto.GetExtension(opts, procframeoptionsv1.E_Proc)
	proc, ok := ext.(*procframeoptionsv1.ProcOptions)
	if !ok || proc == nil {
		return
	}
	if proc.CliPath != nil {
		info.Path = proc.CliPath.Segments
	}
	if proc.Cli != nil {
		info.CLI = *proc.Cli
	}
	if proc.Summary != nil {
		info.Summary = *proc.Summary
	}
	if proc.Hidden != nil {
		info.Hidden = *proc.Hidden
	}
}
