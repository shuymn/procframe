package codegen

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestValidateConfigInfo(t *testing.T) {
	t.Parallel()

	t.Run("valid config info", func(t *testing.T) {
		t.Parallel()
		cfg := &configInfo{
			FilePath: "testdata/proto/test/v1/config.proto",
			Message: &configMessageInfo{
				GoName: "RuntimeConfig",
				Fields: []*configFieldInfo{
					{
						ProtoName:    "log_level",
						GoName:       "LogLevel",
						Kind:         protoreflect.EnumKind,
						EnumTypeName: "LogLevel",
						EnumValues: []enumValueInfo{
							{ProtoName: "LOG_LEVEL_UNSPECIFIED", Number: 0},
							{ProtoName: "LOG_LEVEL_INFO", Number: 1},
							{ProtoName: "LOG_LEVEL_DEBUG", Number: 2},
						},
						HasDefault: true,
						Default:    "info",
						HasEnv:     true,
						Env:        "LOG_LEVEL",
						Bootstrap:  true,
					},
					{
						ProtoName: "token",
						GoName:    "Token",
						Kind:      protoreflect.StringKind,
						HasEnv:    true,
						Env:       "APP_TOKEN",
						Required:  true,
						Secret:    true,
					},
				},
			},
		}
		if err := validateConfigInfo(cfg, &Params{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("non config proto is ignored", func(t *testing.T) {
		t.Parallel()
		cfg := &configInfo{
			FilePath: "testdata/proto/test/v1/service_test.proto",
			Message: &configMessageInfo{
				GoName: "RuntimeConfig",
				Fields: []*configFieldInfo{
					{
						ProtoName: "bad_list",
						GoName:    "BadList",
						Kind:      protoreflect.StringKind,
						IsList:    true,
					},
				},
			},
		}
		if err := validateConfigInfo(cfg, &Params{}); err != nil {
			t.Fatalf("want ignored, got %v", err)
		}
	})

	t.Run("custom config_proto validates matching file", func(t *testing.T) {
		t.Parallel()
		cfg := &configInfo{
			FilePath: "my/custom/settings.proto",
			Message: &configMessageInfo{
				GoName: "RuntimeConfig",
				Fields: []*configFieldInfo{
					{
						ProtoName: "token",
						GoName:    "Token",
						Kind:      protoreflect.StringKind,
						HasEnv:    true,
						Env:       "APP_TOKEN",
					},
				},
			},
		}
		params := &Params{ConfigProto: "my/custom/settings.proto"}
		if err := validateConfigInfo(cfg, params); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("custom config_proto skips non-matching file", func(t *testing.T) {
		t.Parallel()
		cfg := &configInfo{
			FilePath: "other/settings.proto",
			Message: &configMessageInfo{
				GoName: "RuntimeConfig",
				Fields: []*configFieldInfo{
					{
						ProtoName: "bad_list",
						GoName:    "BadList",
						Kind:      protoreflect.StringKind,
						IsList:    true,
					},
				},
			},
		}
		params := &Params{ConfigProto: "my/custom/settings.proto"}
		if err := validateConfigInfo(cfg, params); err != nil {
			t.Fatalf("want ignored, got %v", err)
		}
	})

	t.Run("allow complex field without scalar options", func(t *testing.T) {
		t.Parallel()
		cfg := &configInfo{
			FilePath: "config.proto",
			Message: &configMessageInfo{
				GoName: "RuntimeConfig",
				Fields: []*configFieldInfo{
					{
						ProtoName: "tags",
						GoName:    "Tags",
						Kind:      protoreflect.StringKind,
						IsList:    true,
					},
					{
						ProtoName: "metadata",
						GoName:    "Metadata",
						Kind:      protoreflect.MessageKind,
						IsMap:     true,
					},
					{
						ProtoName: "nested",
						GoName:    "Nested",
						Kind:      protoreflect.MessageKind,
					},
				},
			},
		}
		if err := validateConfigInfo(cfg, &Params{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reject complex field with env", func(t *testing.T) {
		t.Parallel()
		cfg := &configInfo{
			FilePath: "config.proto",
			Message: &configMessageInfo{
				GoName: "RuntimeConfig",
				Fields: []*configFieldInfo{
					{
						ProtoName: "tags",
						GoName:    "Tags",
						Kind:      protoreflect.StringKind,
						IsList:    true,
						HasEnv:    true,
						Env:       "TAGS",
					},
				},
			},
		}
		err := validateConfigInfo(cfg, &Params{})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "cannot use env") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reject complex field with bootstrap", func(t *testing.T) {
		t.Parallel()
		cfg := &configInfo{
			FilePath: "config.proto",
			Message: &configMessageInfo{
				GoName: "RuntimeConfig",
				Fields: []*configFieldInfo{
					{
						ProtoName: "metadata",
						GoName:    "Metadata",
						Kind:      protoreflect.MessageKind,
						IsMap:     true,
						Bootstrap: true,
					},
				},
			},
		}
		err := validateConfigInfo(cfg, &Params{})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "cannot use bootstrap") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reject complex field with default_string", func(t *testing.T) {
		t.Parallel()
		cfg := &configInfo{
			FilePath: "config.proto",
			Message: &configMessageInfo{
				GoName: "RuntimeConfig",
				Fields: []*configFieldInfo{
					{
						ProtoName:  "nested",
						GoName:     "Nested",
						Kind:       protoreflect.MessageKind,
						HasDefault: true,
						Default:    "{}",
					},
				},
			},
		}
		err := validateConfigInfo(cfg, &Params{})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "cannot use default_string") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reject duplicate env", func(t *testing.T) {
		t.Parallel()
		cfg := &configInfo{
			FilePath: "config.proto",
			Message: &configMessageInfo{
				GoName: "RuntimeConfig",
				Fields: []*configFieldInfo{
					{
						ProtoName: "token",
						GoName:    "Token",
						Kind:      protoreflect.StringKind,
						HasEnv:    true,
						Env:       "APP_TOKEN",
					},
					{
						ProtoName: "secret",
						GoName:    "Secret",
						Kind:      protoreflect.StringKind,
						HasEnv:    true,
						Env:       "APP_TOKEN",
					},
				},
			},
		}
		err := validateConfigInfo(cfg, &Params{})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "duplicates") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reject reserved bootstrap flag", func(t *testing.T) {
		t.Parallel()
		cfg := &configInfo{
			FilePath: "config.proto",
			Message: &configMessageInfo{
				GoName: "RuntimeConfig",
				Fields: []*configFieldInfo{
					{
						ProtoName: "config",
						GoName:    "Config",
						Kind:      protoreflect.StringKind,
						Bootstrap: true,
					},
				},
			},
		}
		err := validateConfigInfo(cfg, &Params{})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reject duplicate bootstrap flag", func(t *testing.T) {
		t.Parallel()
		cfg := &configInfo{
			FilePath: "config.proto",
			Message: &configMessageInfo{
				GoName: "RuntimeConfig",
				Fields: []*configFieldInfo{
					{
						ProtoName: "log_level",
						GoName:    "LogLevel",
						Kind:      protoreflect.StringKind,
						Bootstrap: true,
					},
					{
						ProtoName: "log-level",
						GoName:    "LogLevel2",
						Kind:      protoreflect.StringKind,
						Bootstrap: true,
					},
				},
			},
		}
		err := validateConfigInfo(cfg, &Params{})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "duplicates") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reject invalid default", func(t *testing.T) {
		t.Parallel()
		cfg := &configInfo{
			FilePath: "config.proto",
			Message: &configMessageInfo{
				GoName: "RuntimeConfig",
				Fields: []*configFieldInfo{
					{
						ProtoName:  "port",
						GoName:     "Port",
						Kind:       protoreflect.Int32Kind,
						HasDefault: true,
						Default:    "x",
					},
				},
			},
		}
		err := validateConfigInfo(cfg, &Params{})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "default_string") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reject invalid enum default", func(t *testing.T) {
		t.Parallel()
		cfg := &configInfo{
			FilePath: "config.proto",
			Message: &configMessageInfo{
				GoName: "RuntimeConfig",
				Fields: []*configFieldInfo{
					{
						ProtoName:    "log_level",
						GoName:       "LogLevel",
						Kind:         protoreflect.EnumKind,
						EnumTypeName: "LogLevel",
						EnumValues: []enumValueInfo{
							{ProtoName: "LOG_LEVEL_UNSPECIFIED", Number: 0},
							{ProtoName: "LOG_LEVEL_INFO", Number: 1},
						},
						HasDefault: true,
						Default:    "verbose",
					},
				},
			},
		}
		err := validateConfigInfo(cfg, &Params{})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "invalid LogLevel value") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestValidateConfigCollisions(t *testing.T) {
	t.Parallel()

	t.Run("cross-file env collision detected", func(t *testing.T) {
		t.Parallel()

		envSeen := map[string]string{}
		bootstrapSeen := map[string]string{}

		// Simulate first file's field
		field1 := &configFieldInfo{
			ProtoName: "name",
			GoName:    "Name",
			Kind:      protoreflect.StringKind,
			HasEnv:    true,
			Env:       "APP_NAME",
		}
		if err := validateConfigEnv("ConfigA.name", field1, envSeen); err != nil {
			t.Fatalf("unexpected error from first file: %v", err)
		}
		if err := validateConfigBootstrap("ConfigA.name", field1, bootstrapSeen); err != nil {
			t.Fatalf("unexpected error from first file: %v", err)
		}

		// Simulate second file's field with same env
		field2 := &configFieldInfo{
			ProtoName: "title",
			GoName:    "Title",
			Kind:      protoreflect.StringKind,
			HasEnv:    true,
			Env:       "APP_NAME",
		}
		err := validateConfigEnv("ConfigB.title", field2, envSeen)
		if err == nil {
			t.Fatal("expected cross-file env collision error")
		}
		if !strings.Contains(err.Error(), "duplicates") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("cross-file bootstrap collision detected", func(t *testing.T) {
		t.Parallel()

		envSeen := map[string]string{}
		bootstrapSeen := map[string]string{}

		field1 := &configFieldInfo{
			ProtoName: "log_level",
			GoName:    "LogLevel",
			Kind:      protoreflect.StringKind,
			Bootstrap: true,
		}
		if err := validateConfigEnv("ConfigA.log_level", field1, envSeen); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := validateConfigBootstrap("ConfigA.log_level", field1, bootstrapSeen); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Same flag name from different file
		field2 := &configFieldInfo{
			ProtoName: "log_level",
			GoName:    "LogLevel",
			Kind:      protoreflect.StringKind,
			Bootstrap: true,
		}
		err := validateConfigBootstrap("ConfigB.log_level", field2, bootstrapSeen)
		if err == nil {
			t.Fatal("expected cross-file bootstrap collision error")
		}
		if !strings.Contains(err.Error(), "duplicates") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("no collision across files with unique names", func(t *testing.T) {
		t.Parallel()

		envSeen := map[string]string{}
		bootstrapSeen := map[string]string{}

		field1 := &configFieldInfo{
			ProtoName: "prefix",
			GoName:    "Prefix",
			Kind:      protoreflect.StringKind,
			HasEnv:    true,
			Env:       "APP_PREFIX",
			Bootstrap: true,
		}
		if err := validateConfigEnv("ConfigA.prefix", field1, envSeen); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := validateConfigBootstrap("ConfigA.prefix", field1, bootstrapSeen); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		field2 := &configFieldInfo{
			ProtoName: "suffix",
			GoName:    "Suffix",
			Kind:      protoreflect.StringKind,
			HasEnv:    true,
			Env:       "APP_SUFFIX",
			Bootstrap: true,
		}
		if err := validateConfigEnv("ConfigB.suffix", field2, envSeen); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := validateConfigBootstrap("ConfigB.suffix", field2, bootstrapSeen); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestConfigFieldInfoNeedsJSONFieldParser(t *testing.T) {
	t.Parallel()

	t.Run("singular enum needs parser", func(t *testing.T) {
		t.Parallel()

		field := &configFieldInfo{Kind: protoreflect.EnumKind}
		if !field.NeedsJSONFieldParser() {
			t.Fatal("want singular enum to need JSON field parser")
		}
	})

	t.Run("repeated enum does not need parser", func(t *testing.T) {
		t.Parallel()

		field := &configFieldInfo{Kind: protoreflect.EnumKind, IsList: true}
		if field.NeedsJSONFieldParser() {
			t.Fatal("repeated enum should not need JSON field parser")
		}
	})

	t.Run("map enum does not need parser", func(t *testing.T) {
		t.Parallel()

		field := &configFieldInfo{Kind: protoreflect.EnumKind, IsMap: true}
		if field.NeedsJSONFieldParser() {
			t.Fatal("map enum should not need JSON field parser")
		}
	})

	t.Run("scalar non-enum does not need parser", func(t *testing.T) {
		t.Parallel()

		field := &configFieldInfo{Kind: protoreflect.StringKind}
		if field.NeedsJSONFieldParser() {
			t.Fatal("non-enum should not need JSON field parser")
		}
	})
}

func TestErrMultipleConfigMessages(t *testing.T) {
	t.Parallel()

	err := errMultipleConfigMessages("foo/config.proto", []configMessageInfo{
		{GoName: "BConfig"},
		{GoName: "AConfig"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "AConfig, BConfig") {
		t.Fatalf("unexpected error: %v", err)
	}
}
