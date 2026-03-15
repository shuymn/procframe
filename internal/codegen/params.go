package codegen

import (
	"fmt"
	"path"
)

const defaultConfigProtoBasename = "config.proto"

// Params holds plugin parameters parsed from protoc --<plugin>_opt flags.
type Params struct {
	ConfigProto string
}

// isConfigProto reports whether protoPath matches the config proto pattern.
//
// When ConfigProto is empty (default), it matches any file whose base name
// equals "config.proto". When ConfigProto contains a slash, an exact path
// match is required. Otherwise a base-name match is performed.
func (p *Params) isConfigProto(protoPath string) bool {
	pattern := p.ConfigProto
	if pattern == "" {
		return path.Base(protoPath) == defaultConfigProtoBasename
	}
	if containsSlash(pattern) {
		return protoPath == pattern
	}
	return path.Base(protoPath) == pattern
}

// Set implements the flag.Value-style callback expected by protogen.Options.ParamFunc.
func (p *Params) Set(name, value string) error {
	switch name {
	case "config_proto":
		p.ConfigProto = value
		return nil
	default:
		return fmt.Errorf("unknown parameter %q", name)
	}
}

func containsSlash(s string) bool {
	for _, c := range s {
		if c == '/' {
			return true
		}
	}
	return false
}
