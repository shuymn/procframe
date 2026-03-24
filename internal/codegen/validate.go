package codegen

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/shuymn/procframe/config"
)

// validateDuplicatePaths checks that no two methods resolve to the same
// effective CLI path.
func validateDuplicatePaths(services []serviceInfo) error {
	seen := make(map[string]string)
	for _, svc := range services {
		for _, m := range svc.Methods {
			if !m.CLI {
				continue
			}
			path := strings.Join(slices.Concat(svc.Path, m.Path), " ")
			if prev, dup := seen[path]; dup {
				return fmt.Errorf("duplicate CLI path %q: %s and %s", path, prev, m.FullName)
			}
			seen[path] = m.FullName
		}
	}
	return nil
}

// validateEnumCollisions checks all enum types used in request messages
// for stripped value collisions.
func validateEnumCollisions(plugin *protogen.Plugin) error {
	checked := make(map[string]bool)
	for _, f := range plugin.Files {
		if !f.Generate {
			continue
		}
		for _, msg := range f.Messages {
			if err := checkMessageEnums(msg, checked); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkMessageEnums(msg *protogen.Message, checked map[string]bool) error {
	for _, field := range msg.Fields {
		if field.Enum == nil {
			continue
		}
		enumName := string(field.Enum.Desc.FullName())
		if checked[enumName] {
			continue
		}
		checked[enumName] = true

		values := make([]enumValueInfo, 0, len(field.Enum.Values))
		for _, v := range field.Enum.Values {
			values = append(values, enumValueInfo{
				ProtoName: string(v.Desc.Name()),
				Number:    int32(v.Desc.Number()),
			})
		}
		typeName := string(field.Enum.Desc.Name())
		if _, err := enumCLIValues(typeName, values); err != nil {
			return err
		}
	}
	return nil
}

// validateBindInto checks that bind_into fields exist in all descendant
// request messages and are message-typed.
func validateBindInto(services []serviceInfo, plugin *protogen.Plugin) error {
	for _, svc := range services {
		if svc.BindInto == "" {
			continue
		}
		for _, m := range svc.Methods {
			if !m.CLI {
				continue
			}
			if err := validateBindIntoMethod(svc.BindInto, m, plugin); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateBindIntoMethod checks a single method's request message for the
// bind_into field.
func validateBindIntoMethod(bindInto string, m methodInfo, plugin *protogen.Plugin) error {
	msg := findMessage(plugin, m.InputGoName)
	if msg == nil {
		return nil
	}
	field := findField(msg, bindInto)
	if field == nil {
		return fmt.Errorf(
			"bind_into %q: field not found in %s (method %s)",
			bindInto, m.InputGoName, m.FullName,
		)
	}
	if field.Desc.Kind() != protoreflect.MessageKind {
		return fmt.Errorf(
			"bind_into %q: field in %s must be a message type, got %s",
			bindInto, m.InputGoName, field.Desc.Kind(),
		)
	}
	return nil
}

func findMessage(plugin *protogen.Plugin, goName string) *protogen.Message {
	for _, f := range plugin.Files {
		if !f.Generate {
			continue
		}
		for _, msg := range f.Messages {
			if msg.GoIdent.GoName == goName {
				return msg
			}
		}
	}
	return nil
}

func findField(msg *protogen.Message, protoName string) *protogen.Field {
	for _, field := range msg.Fields {
		if string(field.Desc.Name()) == protoName {
			return field
		}
	}
	return nil
}

func validateConfigInfo(cfg *configInfo, params *Params) error {
	if !shouldValidateConfig(cfg, params) {
		return nil
	}

	envSeen := map[string]string{}
	bootstrapSeen := map[string]string{}

	for _, field := range cfg.Message.Fields {
		fieldPath := cfg.Message.GoName + "." + field.ProtoName
		if err := validateConfigFieldType(fieldPath, field); err != nil {
			return err
		}
		if err := validateConfigFieldMetadata(fieldPath, field, envSeen, bootstrapSeen); err != nil {
			return err
		}
		if field.HasDefault {
			if err := validateConfigDefault(fieldPath, field); err != nil {
				return err
			}
		}
	}

	return nil
}

func shouldValidateConfig(cfg *configInfo, params *Params) bool {
	return cfg != nil &&
		params.isConfigProto(cfg.FilePath) &&
		len(cfg.Message.Fields) > 0
}

func validateConfigFieldMetadata(
	fieldPath string,
	field *configFieldInfo,
	envSeen map[string]string,
	bootstrapSeen map[string]string,
) error {
	if err := validateConfigEnv(fieldPath, field, envSeen); err != nil {
		return err
	}
	if err := validateConfigBootstrap(fieldPath, field, bootstrapSeen); err != nil {
		return err
	}
	return nil
}

func validateConfigEnv(
	fieldPath string,
	field *configFieldInfo,
	envSeen map[string]string,
) error {
	if !field.HasEnv {
		return nil
	}
	if field.Env == "" {
		return fmt.Errorf("config field %s: env must not be empty", fieldPath)
	}
	if prev, dup := envSeen[field.Env]; dup {
		return fmt.Errorf(
			"config field %s: env %q duplicates %s",
			fieldPath, field.Env, prev,
		)
	}
	envSeen[field.Env] = fieldPath
	return nil
}

func validateConfigBootstrap(
	fieldPath string,
	field *configFieldInfo,
	bootstrapSeen map[string]string,
) error {
	if !field.Bootstrap {
		return nil
	}

	flagName := field.FlagName()
	if flagName == config.ReservedConfigFlag {
		return fmt.Errorf(
			"config field %s: bootstrap flag --%s is reserved",
			fieldPath, config.ReservedConfigFlag,
		)
	}
	if prev, dup := bootstrapSeen[flagName]; dup {
		return fmt.Errorf(
			"config field %s: bootstrap flag --%s duplicates %s",
			fieldPath, flagName, prev,
		)
	}
	bootstrapSeen[flagName] = fieldPath
	return nil
}

func errMultipleConfigMessages(filePath string, msgs []configMessageInfo) error {
	names := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		names = append(names, msg.GoName)
	}
	slices.Sort(names)
	return fmt.Errorf(
		"%s: exactly one top-level config message with field options is allowed; found %s",
		filePath, strings.Join(names, ", "),
	)
}

// validateConfigCollisions checks env var and bootstrap flag collisions
// across all config messages in the generation request.
func validateConfigCollisions(plugin *protogen.Plugin, params *Params) error {
	envSeen := map[string]string{}
	bootstrapSeen := map[string]string{}

	for _, f := range plugin.Files {
		if !f.Generate {
			continue
		}
		cfgInfo, err := extractConfigInfo(f, params)
		if err != nil || cfgInfo == nil {
			continue
		}
		for _, field := range cfgInfo.Message.Fields {
			fieldPath := cfgInfo.Message.GoName + "." + field.ProtoName
			if err := validateConfigEnv(fieldPath, field, envSeen); err != nil {
				return err
			}
			if err := validateConfigBootstrap(fieldPath, field, bootstrapSeen); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateConfigFieldType(fieldPath string, field *configFieldInfo) error {
	if isComplexConfigField(field) {
		return validateComplexConfigFieldOptions(fieldPath, field)
	}
	// Scalar and bytes fields are always valid.
	return nil
}

// isComplexConfigField returns true for fields that cannot use
// env/bootstrap/default_string (list, map, message, group).
func isComplexConfigField(f *configFieldInfo) bool {
	return f.IsList || f.IsMap ||
		f.Kind == protoreflect.MessageKind ||
		f.Kind == protoreflect.GroupKind
}

// validateComplexConfigFieldOptions ensures that complex fields do not
// carry scalar-only options (env, bootstrap, default_string).
func validateComplexConfigFieldOptions(fieldPath string, f *configFieldInfo) error {
	if f.HasEnv {
		return fmt.Errorf(
			"config field %s: complex field (list/map/message) cannot use env",
			fieldPath,
		)
	}
	if f.Bootstrap {
		return fmt.Errorf(
			"config field %s: complex field (list/map/message) cannot use bootstrap",
			fieldPath,
		)
	}
	if f.HasDefault {
		return fmt.Errorf(
			"config field %s: complex field (list/map/message) cannot use default_string",
			fieldPath,
		)
	}
	return nil
}

func validateConfigDefault(fieldPath string, field *configFieldInfo) error {
	if _, err := parseConfigValueForValidation(field, field.Default); err != nil {
		return fmt.Errorf(
			"config field %s: default_string %q is invalid: %w",
			fieldPath, field.Default, err,
		)
	}
	return nil
}

func parseConfigValueForValidation(field *configFieldInfo, raw string) (any, error) {
	switch field.Kind {
	case protoreflect.BoolKind:
		return config.ParseBool(raw)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return config.ParseInt32(raw)
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return config.ParseInt64(raw)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return config.ParseUint32(raw)
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return config.ParseUint64(raw)
	case protoreflect.FloatKind:
		return config.ParseFloat32(raw)
	case protoreflect.DoubleKind:
		return config.ParseFloat64(raw)
	case protoreflect.StringKind:
		return config.ParseString(raw)
	case protoreflect.BytesKind:
		return config.ParseBytes(raw)
	case protoreflect.EnumKind:
		if v, err := strconv.ParseInt(raw, 10, 32); err == nil {
			return int32(v), nil
		}

		mappings, err := enumCLIValues(field.EnumTypeName, field.EnumValues)
		if err != nil {
			return nil, err
		}
		enumMappings := make([]*config.EnumMapping, 0, len(mappings))
		for _, m := range mappings {
			enumMappings = append(enumMappings, &config.EnumMapping{
				Name:   m.CLIValue,
				Number: m.Number,
			})
		}
		return config.ParseEnum(raw, enumMappings, field.EnumTypeName)
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return nil, fmt.Errorf("unsupported kind %s", field.Kind)
	default:
		return nil, fmt.Errorf("unsupported kind %s", field.Kind)
	}
}
