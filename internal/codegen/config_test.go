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
						EnumValues: []*enumValueInfo{
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

	t.Run("reject repeated or map", func(t *testing.T) {
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
				},
			},
		}
		err := validateConfigInfo(cfg, &Params{})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "repeated/map fields are not supported") {
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
						EnumValues: []*enumValueInfo{
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

func TestErrMultipleConfigMessages(t *testing.T) {
	t.Parallel()

	err := errMultipleConfigMessages("foo/config.proto", []*configMessageInfo{
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
