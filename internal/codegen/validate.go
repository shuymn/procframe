package codegen

import (
	"fmt"
	"slices"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
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
