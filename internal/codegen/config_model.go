package codegen

import (
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type configInfo struct {
	FilePath string
	Message  *configMessageInfo
}

type configMessageInfo struct {
	GoName string
	Fields []*configFieldInfo
}

type configFieldInfo struct {
	ProtoName string
	JSONName  string
	GoName    string

	Kind   protoreflect.Kind
	IsList bool
	IsMap  bool

	EnumTypeName string
	EnumGoIdent  protogen.GoIdent
	EnumValues   []*enumValueInfo

	Env        string
	HasEnv     bool
	Default    string
	HasDefault bool
	Required   bool
	Secret     bool
	Bootstrap  bool
}

func (f *configFieldInfo) FlagName() string {
	return fieldToFlagName(f.ProtoName)
}

func (f *configFieldInfo) NeedsStringParser() bool {
	return f.HasDefault || f.HasEnv || f.Bootstrap
}
