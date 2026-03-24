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
		CLI:   true, // default
		Shape: methodShape(m),
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
		info.CLI = proc.Cli.GetEnabled()
	}
	if proc.Connect != nil {
		info.Connect = proc.Connect.GetEnabled()
	}
	if proc.Ws != nil {
		info.Ws = proc.Ws.GetEnabled()
	}
	if proc.Summary != nil {
		info.Summary = *proc.Summary
	}
	if proc.Hidden != nil {
		info.Hidden = *proc.Hidden
	}
}

// Shape constants used by codegen to distinguish RPC shapes.
const (
	shapeUnary        = "unary"
	shapeClientStream = "client_stream"
	shapeServerStream = "server_stream"
	shapeBidi         = "bidi"
)

func methodShape(m *protogen.Method) string {
	cs := m.Desc.IsStreamingClient()
	ss := m.Desc.IsStreamingServer()
	switch {
	case cs && ss:
		return shapeBidi
	case cs:
		return shapeClientStream
	case ss:
		return shapeServerStream
	default:
		return shapeUnary
	}
}

func extractConfigInfo(file *protogen.File, params *Params) (*configInfo, error) {
	if !params.isConfigProto(file.Desc.Path()) {
		return nil, nil
	}

	candidates := collectConfigCandidates(file.Messages)
	if len(candidates) == 0 {
		return nil, nil
	}
	if len(candidates) > 1 {
		return nil, errMultipleConfigMessages(file.Desc.Path(), candidates)
	}
	return &configInfo{
		FilePath: file.Desc.Path(),
		Message:  &candidates[0],
	}, nil
}

func collectConfigCandidates(messages []*protogen.Message) []configMessageInfo {
	candidates := make([]configMessageInfo, 0, len(messages))
	for _, msg := range messages {
		info, ok := extractConfigMessageInfo(msg)
		if !ok {
			continue
		}
		candidates = append(candidates, info)
	}
	return candidates
}

func extractConfigMessageInfo(msg *protogen.Message) (configMessageInfo, bool) {
	fields := make([]*configFieldInfo, 0, len(msg.Fields))
	for _, field := range msg.Fields {
		cfgOpt, ok := getConfigFieldOptions(field)
		if !ok {
			continue
		}
		fields = append(fields, extractConfigFieldInfo(field, cfgOpt))
	}
	if len(fields) == 0 {
		return configMessageInfo{}, false
	}
	return configMessageInfo{
		GoName: msg.GoIdent.GoName,
		Fields: fields,
	}, true
}

func extractConfigFieldInfo(
	field *protogen.Field,
	cfgOpt *procframeoptionsv1.ConfigFieldOptions,
) *configFieldInfo {
	info := &configFieldInfo{
		ProtoName: string(field.Desc.Name()),
		JSONName:  field.Desc.JSONName(),
		GoName:    field.GoName,
		Kind:      field.Desc.Kind(),
		IsList:    field.Desc.IsList(),
		IsMap:     field.Desc.IsMap(),
		Required:  cfgOpt.GetRequired(),
		Secret:    cfgOpt.GetSecret(),
		Bootstrap: cfgOpt.GetBootstrap(),
	}
	if cfgOpt.Env != nil {
		info.HasEnv = true
		info.Env = cfgOpt.GetEnv()
	}
	if cfgOpt.DefaultString != nil {
		info.HasDefault = true
		info.Default = cfgOpt.GetDefaultString()
	}
	if field.Enum != nil {
		info.EnumTypeName = string(field.Enum.Desc.Name())
		info.EnumGoIdent = field.Enum.GoIdent
		info.EnumValues = extractEnumValueInfos(field.Enum)
	}
	return info
}

func extractEnumValueInfos(enum *protogen.Enum) []enumValueInfo {
	values := make([]enumValueInfo, 0, len(enum.Values))
	for _, v := range enum.Values {
		values = append(values, enumValueInfo{
			ProtoName: string(v.Desc.Name()),
			Number:    int32(v.Desc.Number()),
		})
	}
	return values
}

func getConfigFieldOptions(field *protogen.Field) (*procframeoptionsv1.ConfigFieldOptions, bool) {
	opts := field.Desc.Options()
	if opts == nil {
		return nil, false
	}
	ext := proto.GetExtension(opts, procframeoptionsv1.E_Config)
	cfgOpt, ok := ext.(*procframeoptionsv1.ConfigFieldOptions)
	if !ok || cfgOpt == nil {
		return nil, false
	}
	return cfgOpt, true
}

func getFieldDescription(field *protogen.Field) string {
	opts := field.Desc.Options()
	if opts == nil {
		return ""
	}
	ext := proto.GetExtension(opts, procframeoptionsv1.E_Field)
	fieldOpt, ok := ext.(*procframeoptionsv1.FieldOptions)
	if !ok || fieldOpt == nil {
		return ""
	}
	return fieldOpt.GetDescription()
}
