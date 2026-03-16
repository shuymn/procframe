package codegen

import (
	"fmt"
	"strconv"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var (
	configPkg  = protogen.GoImportPath("github.com/shuymn/procframe/config")
	protoPkg   = protogen.GoImportPath("google.golang.org/protobuf/proto")
	strconvPkg = protogen.GoImportPath("strconv")
)

func generateConfig(g *protogen.GeneratedFile, cfg *configInfo) error {
	emitPresenceState(g, cfg)
	emitLoadConfigFile(g, cfg)
	emitApplyDefaults(g, cfg)
	emitApplyEnvWith(g, cfg)
	emitApplyBootstrap(g, cfg)
	emitValidateRequired(g, cfg)
	emitConfigSpec(g, cfg)
	emitFormatter(g, cfg)

	if err := emitFieldParsers(g, cfg); err != nil {
		return err
	}

	return nil
}

func emitPresenceState(g *protogen.GeneratedFile, cfg *configInfo) {
	g.P("type runtimeConfigFieldPresence struct {")
	for _, field := range cfg.Message.Fields {
		if !field.Required {
			continue
		}
		g.P("\t", field.GoName, " bool")
	}
	g.P("}")
	g.P()
}

func emitLoadConfigFile(g *protogen.GeneratedFile, cfg *configInfo) {
	g.P(
		"func loadRuntimeConfigFile(cfg *",
		cfg.Message.GoName,
		", presence *runtimeConfigFieldPresence, filePath string) error {",
	)
	secretFields := make([]string, 0, len(cfg.Message.Fields))
	requiredFields := make([]*configFieldInfo, 0, len(cfg.Message.Fields))
	for _, field := range cfg.Message.Fields {
		if field.Secret {
			secretFields = append(secretFields, field.JSONName)
		}
		if field.Required {
			requiredFields = append(requiredFields, field)
		}
	}
	args := []any{"\treturn ", configPkg.Ident("MergeJSONFile"), "(filePath, cfg"}
	for _, fieldName := range secretFields {
		args = append(args, ", ", strconv.Quote(fieldName))
	}
	args = append(args, ")")
	loadArgs := append([]any{"\tpresentFields, err := "}, args[1:]...)
	g.P(loadArgs...)
	g.P("\tif err != nil {")
	g.P("\t\treturn err")
	g.P("\t}")
	if len(requiredFields) == 0 {
		g.P("\t_ = presentFields")
	}
	for _, field := range requiredFields {
		g.P("\tif _, ok := presentFields[", strconv.Quote(field.JSONName), "]; ok {")
		g.P("\t\tpresence.", field.GoName, " = true")
		g.P("\t}")
	}
	g.P("\treturn nil")
	g.P("}")
	g.P()
}

func emitApplyDefaults(g *protogen.GeneratedFile, cfg *configInfo) {
	g.P(
		"func applyRuntimeConfigDefaults(cfg *",
		cfg.Message.GoName,
		", presence *runtimeConfigFieldPresence) error {",
	)
	for _, field := range cfg.Message.Fields {
		if !field.HasDefault {
			continue
		}
		parser := configFieldParserName(field)
		parsedVar := "parsed" + field.GoName
		g.P("\t", parsedVar, ", err := ", parser, "(", strconv.Quote(field.Default), ")")
		g.P("\tif err != nil {")
		g.P(
			"\t\treturn ",
			fmtPkg.Ident("Errorf"),
			"(\"",
			cfg.Message.GoName,
			".",
			field.ProtoName,
			": %w\", err)",
		)
		g.P("\t}")
		g.P("\tcfg.", field.GoName, " = ", parsedVar)
		if field.Required {
			g.P("\tpresence.", field.GoName, " = true")
		}
	}
	g.P("\treturn nil")
	g.P("}")
	g.P()
}

func emitApplyEnvWith(g *protogen.GeneratedFile, cfg *configInfo) {
	g.P(
		"func applyRuntimeConfigEnvWith(cfg *",
		cfg.Message.GoName,
		", presence *runtimeConfigFieldPresence, lookup ",
		configPkg.Ident("EnvLookup"),
		") error {",
	)
	for _, field := range cfg.Message.Fields {
		if !field.HasEnv {
			continue
		}
		parser := configFieldParserName(field)
		g.P(
			"\tif err := ",
			configPkg.Ident("ApplyEnv"),
			"(lookup, ",
			strconv.Quote(field.Env),
			", func(raw string) error {",
		)
		g.P("\t\t\tv, err := ", parser, "(raw)")
		g.P("\t\t\tif err != nil {")
		if field.Secret {
			g.P("\t\t\t\treturn ", fmtPkg.Ident("Errorf"), "(\"invalid secret value\")")
		} else {
			g.P("\t\t\t\treturn err")
		}
		g.P("\t\t\t}")
		g.P("\t\t\tcfg.", field.GoName, " = v")
		if field.Required {
			g.P("\t\t\tpresence.", field.GoName, " = true")
		}
		g.P("\t\t\treturn nil")
		g.P("\t\t}); err != nil {")
		g.P(
			"\t\treturn ",
			fmtPkg.Ident("Errorf"),
			"(\"",
			cfg.Message.GoName,
			".",
			field.ProtoName,
			" from env ",
			field.Env,
			": %w\", err)",
		)
		g.P("\t\t}")
	}
	g.P("\treturn nil")
	g.P("}")
	g.P()
}

func emitApplyBootstrap(g *protogen.GeneratedFile, cfg *configInfo) {
	g.P(
		"func applyRuntimeConfigBootstrap(cfg *",
		cfg.Message.GoName,
		", presence *runtimeConfigFieldPresence, values map[string]string) error {",
	)
	for _, field := range cfg.Message.Fields {
		if !field.Bootstrap {
			continue
		}
		parser := configFieldParserName(field)
		flagName := field.FlagName()
		g.P("\tif raw, ok := values[", strconv.Quote(flagName), "]; ok {")
		g.P("\t\tv, err := ", parser, "(raw)")
		g.P("\t\tif err != nil {")
		if field.Secret {
			g.P(
				"\t\t\treturn ",
				fmtPkg.Ident("Errorf"),
				"(\"",
				cfg.Message.GoName,
				".",
				field.ProtoName,
				" from --",
				flagName,
				": invalid secret value\")",
			)
		} else {
			g.P(
				"\t\t\treturn ",
				fmtPkg.Ident("Errorf"),
				"(\"",
				cfg.Message.GoName,
				".",
				field.ProtoName,
				" from --",
				flagName,
				": %w\", err)",
			)
		}
		g.P("\t\t}")
		g.P("\t\tcfg.", field.GoName, " = v")
		if field.Required {
			g.P("\t\tpresence.", field.GoName, " = true")
		}
		g.P("\t}")
	}
	g.P("\treturn nil")
	g.P("}")
	g.P()
}

func emitValidateRequired(g *protogen.GeneratedFile, cfg *configInfo) {
	g.P("func validateRuntimeConfigRequired(presence *runtimeConfigFieldPresence) error {")
	for _, field := range cfg.Message.Fields {
		if !field.Required {
			continue
		}
		g.P(
			"\tif err := ",
			configPkg.Ident("ValidateRequired"),
			"(",
			strconv.Quote(cfg.Message.GoName+"."+field.ProtoName),
			", presence.",
			field.GoName,
			"); err != nil {",
		)
		g.P("\t\treturn err")
		g.P("\t}")
	}
	g.P("\treturn nil")
	g.P("}")
	g.P()
}

func emitConfigSpec(g *protogen.GeneratedFile, cfg *configInfo) {
	g.P("// ConfigSpec returns a ", configPkg.Ident("Spec"), " for use with ", configPkg.Ident("Load"), ".")
	g.P("func (x *", cfg.Message.GoName, ") ConfigSpec() *", configPkg.Ident("Spec"), " {")
	g.P("\tpresence := &runtimeConfigFieldPresence{}")
	g.P("\treturn &", configPkg.Ident("Spec"), "{")

	// EnvNames
	g.P("\t\tEnvNames: map[string]string{")
	for _, field := range cfg.Message.Fields {
		if !field.HasEnv {
			continue
		}
		g.P(
			"\t\t\t",
			strconv.Quote(field.Env),
			": ",
			strconv.Quote(cfg.Message.GoName+"."+field.ProtoName),
			",",
		)
	}
	g.P("\t\t},")

	// BootstrapSpecs
	g.P("\t\tBootstrapSpecs: []*", configPkg.Ident("BootstrapSpec"), "{")
	for _, field := range cfg.Message.Fields {
		if !field.Bootstrap {
			continue
		}
		g.P("\t\t\t{Flag: ", strconv.Quote(field.FlagName()), "},")
	}
	g.P("\t\t},")

	// ApplyDefaults
	g.P("\t\tApplyDefaults: func() error {")
	g.P("\t\t\treturn applyRuntimeConfigDefaults(x, presence)")
	g.P("\t\t},")

	// ApplyConfigFile
	g.P("\t\tApplyConfigFile: func(path string) error {")
	g.P("\t\t\treturn loadRuntimeConfigFile(x, presence, path)")
	g.P("\t\t},")

	// ApplyEnv
	g.P("\t\tApplyEnv: func(lookup ", configPkg.Ident("EnvLookup"), ") error {")
	g.P("\t\t\treturn applyRuntimeConfigEnvWith(x, presence, lookup)")
	g.P("\t\t},")

	// ApplyBootstrap
	g.P("\t\tApplyBootstrap: func(values map[string]string) error {")
	g.P("\t\t\treturn applyRuntimeConfigBootstrap(x, presence, values)")
	g.P("\t\t},")

	// ValidateRequired
	g.P("\t\tValidateRequired: func() error {")
	g.P("\t\t\treturn validateRuntimeConfigRequired(presence)")
	g.P("\t\t},")

	g.P("\t}")
	g.P("}")
	g.P()
}

func emitFormatter(g *protogen.GeneratedFile, cfg *configInfo) {
	formatViewType := unexportedTypeName(cfg.Message.GoName + "FormatView")
	g.P("type ", formatViewType, " ", cfg.Message.GoName)
	g.P()
	g.P("// Format masks secret fields when the config is formatted.")
	g.P("func (x *", cfg.Message.GoName, ") Format(state ", fmtPkg.Ident("State"), ", verb rune) {")
	g.P("\tif x == nil {")
	g.P("\t\t_, _ = ", fmtPkg.Ident("Fprint"), "(state, \"<nil>\")")
	g.P("\t\treturn")
	g.P("\t}")
	g.P("\tmasked := ", protoPkg.Ident("Clone"), "(x).(*", cfg.Message.GoName, ")")
	emitFormatterMasking(g, cfg)
	g.P("\tformatted := formatMasked", cfg.Message.GoName, "(masked, state)")
	g.P("\tformatVerb := resolveMaskedFormatVerb(verb)")
	g.P(
		"\t_, _ = ",
		fmtPkg.Ident("Fprintf"),
		"(state, ",
		fmtPkg.Ident("FormatString"),
		"(state, formatVerb), formatted)",
	)
	g.P("}")
	g.P()
	emitFormatMaskedHelper(g, cfg, formatViewType)
	emitResolveMaskedFormatVerb(g)
}

func emitFormatterMasking(g *protogen.GeneratedFile, cfg *configInfo) {
	for _, field := range cfg.Message.Fields {
		if !field.Secret {
			continue
		}
		g.P("\tmasked.", field.GoName, " = ", redactedValueExpr(g, field, "masked"))
	}
}

func emitFormatMaskedHelper(
	g *protogen.GeneratedFile,
	cfg *configInfo,
	formatViewType string,
) {
	g.P(
		"func formatMasked",
		cfg.Message.GoName,
		"(masked *",
		cfg.Message.GoName,
		", state ",
		fmtPkg.Ident("State"),
		") string {",
	)
	g.P("\tif state.Flag('+') {")
	g.P("\t\tb, err := ", protojsonPkg.Ident("MarshalOptions"), "{Multiline: true, Indent: \"  \"}.Marshal(masked)")
	g.P("\t\tif err == nil {")
	g.P("\t\t\treturn string(b)")
	g.P("\t\t}")
	g.P("\t}")
	g.P("\tb, err := ", protojsonPkg.Ident("MarshalOptions"), "{}.Marshal(masked)")
	g.P("\tif err == nil {")
	g.P("\t\treturn string(b)")
	g.P("\t}")
	g.P("\treturn ", fmtPkg.Ident("Sprint"), "((*", formatViewType, ")(masked))")
	g.P("}")
	g.P()
}

func emitResolveMaskedFormatVerb(g *protogen.GeneratedFile) {
	g.P("func resolveMaskedFormatVerb(verb rune) rune {")
	g.P("\tswitch verb {")
	g.P("\tcase 'q', 'x', 'X', 's':")
	g.P("\t\treturn verb")
	g.P("\tdefault:")
	g.P("\t\treturn 's'")
	g.P("\t}")
	g.P("}")
	g.P()
}

func emitFieldParsers(g *protogen.GeneratedFile, cfg *configInfo) error {
	for _, field := range cfg.Message.Fields {
		if !field.NeedsStringParser() {
			continue
		}
		if err := emitFieldParser(g, field); err != nil {
			return err
		}
	}
	return nil
}

func emitFieldParser(g *protogen.GeneratedFile, field *configFieldInfo) error {
	funcName := configFieldParserName(field)
	goType := configFieldGoType(g, field)
	g.P("func ", funcName, "(raw string) (", goType, ", error) {")

	if field.Kind == protoreflect.EnumKind {
		if err := emitEnumFieldParserBody(g, field); err != nil {
			return err
		}
	} else {
		parserName, ok := scalarConfigParserName(field.Kind)
		if !ok {
			return fmt.Errorf("unsupported kind %s", field.Kind)
		}
		g.P("\treturn ", configPkg.Ident(parserName), "(raw)")
	}

	g.P("}")
	g.P()
	return nil
}

func emitEnumFieldParserBody(g *protogen.GeneratedFile, field *configFieldInfo) error {
	mappings, err := enumCLIValues(field.EnumTypeName, field.EnumValues)
	if err != nil {
		return fmt.Errorf("%s: %w", field.ProtoName, err)
	}
	g.P("\tmappings := []*", configPkg.Ident("EnumMapping"), "{")
	for _, m := range mappings {
		g.P("\t\t&", configPkg.Ident("EnumMapping"), "{Name: ", strconv.Quote(m.CLIValue), ", Number: ", m.Number, "},")
	}
	g.P("\t}")
	g.P("\tif v, err := ", strconvPkg.Ident("ParseInt"), "(raw, 10, 32); err == nil {")
	g.P("\t\treturn ", g.QualifiedGoIdent(field.EnumGoIdent), "(v), nil")
	g.P("\t}")
	g.P("\tv, err := ", configPkg.Ident("ParseEnum"), "(raw, mappings, ", strconv.Quote(field.EnumTypeName), ")")
	g.P("\tif err != nil {")
	g.P("\t\treturn 0, err")
	g.P("\t}")
	g.P("\treturn ", g.QualifiedGoIdent(field.EnumGoIdent), "(v), nil")
	return nil
}

func scalarConfigParserName(kind protoreflect.Kind) (string, bool) {
	switch kind {
	case protoreflect.StringKind:
		return "ParseString", true
	case protoreflect.BytesKind:
		return "ParseBytes", true
	case protoreflect.BoolKind:
		return "ParseBool", true
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return "ParseInt32", true
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return "ParseInt64", true
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return "ParseUint32", true
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return "ParseUint64", true
	case protoreflect.FloatKind:
		return "ParseFloat32", true
	case protoreflect.DoubleKind:
		return "ParseFloat64", true
	case protoreflect.EnumKind, protoreflect.MessageKind, protoreflect.GroupKind:
		return "", false
	default:
		return "", false
	}
}

func configFieldParserName(field *configFieldInfo) string {
	return "parseRuntimeConfigField" + field.GoName
}

func configFieldGoType(g *protogen.GeneratedFile, field *configFieldInfo) any {
	switch field.Kind {
	case protoreflect.BoolKind:
		return "bool"
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return "int32"
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return "int64"
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return "uint32"
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return "uint64"
	case protoreflect.FloatKind:
		return "float32"
	case protoreflect.DoubleKind:
		return "float64"
	case protoreflect.StringKind:
		return "string"
	case protoreflect.BytesKind:
		return "[]byte"
	case protoreflect.EnumKind:
		return g.QualifiedGoIdent(field.EnumGoIdent)
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return "any"
	default:
		return "string"
	}
}

func redactedValueExpr(g *protogen.GeneratedFile, field *configFieldInfo, varName string) any {
	redactIfSet := g.QualifiedGoIdent(configPkg.Ident("RedactIfSet"))
	redactBytes := g.QualifiedGoIdent(configPkg.Ident("RedactBytes"))
	placeholder := g.QualifiedGoIdent(configPkg.Ident("RedactedPlaceholder"))
	fieldRef := varName + "." + field.GoName

	switch field.Kind {
	case protoreflect.StringKind:
		return redactIfSet + "(" + fieldRef + ", " + placeholder + ")"
	case protoreflect.BytesKind:
		return redactBytes + "(" + fieldRef + ")"
	case protoreflect.BoolKind:
		return redactIfSet + "(" + fieldRef + ", false)"
	case protoreflect.EnumKind:
		return redactIfSet + "(" + fieldRef + ", " + g.QualifiedGoIdent(field.EnumGoIdent) + "(0))"
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind,
		protoreflect.FloatKind, protoreflect.DoubleKind:
		return redactIfSet + "(" + fieldRef + ", " + zeroLiteralForKind(field) + ")"
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return "nil"
	default:
		return "nil"
	}
}

func zeroLiteralForKind(field *configFieldInfo) string {
	switch field.Kind {
	case protoreflect.BoolKind:
		return "false"
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return "int32(0)"
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return "int64(0)"
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return "uint32(0)"
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return "uint64(0)"
	case protoreflect.FloatKind:
		return "float32(0)"
	case protoreflect.DoubleKind:
		return "float64(0)"
	case protoreflect.StringKind:
		return `""`
	case protoreflect.BytesKind:
		return "nil"
	case protoreflect.EnumKind:
		return "0"
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return "nil"
	default:
		return "0"
	}
}

func unexportedTypeName(name string) string {
	if name == "" {
		return "formatView"
	}
	first := name[0]
	if first >= 'A' && first <= 'Z' {
		first += 'a' - 'A'
	}
	return string(first) + name[1:]
}
