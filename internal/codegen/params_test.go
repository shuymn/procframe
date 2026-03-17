package codegen

import (
	"strings"
	"testing"
)

func TestIsConfigProto(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		configProto string
		protoPath   string
		want        bool
	}{
		{
			name:        "default matches config.proto basename",
			configProto: "",
			protoPath:   "foo/bar/config.proto",
			want:        true,
		},
		{
			name:        "default matches bare config.proto",
			configProto: "",
			protoPath:   "config.proto",
			want:        true,
		},
		{
			name:        "default does not match other basename",
			configProto: "",
			protoPath:   "foo/bar/service.proto",
			want:        false,
		},
		{
			name:        "basename pattern matches any path with that basename",
			configProto: "settings.proto",
			protoPath:   "my/custom/settings.proto",
			want:        true,
		},
		{
			name:        "basename pattern does not match different basename",
			configProto: "settings.proto",
			protoPath:   "my/custom/config.proto",
			want:        false,
		},
		{
			name:        "full path pattern matches exact path",
			configProto: "my/custom/settings.proto",
			protoPath:   "my/custom/settings.proto",
			want:        true,
		},
		{
			name:        "full path pattern does not match different path with same basename",
			configProto: "my/custom/settings.proto",
			protoPath:   "other/settings.proto",
			want:        false,
		},
		{
			name:        "full path pattern does not match substring",
			configProto: "my/custom/settings.proto",
			protoPath:   "prefix/my/custom/settings.proto",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &Params{ConfigProto: tt.configProto}
			if got := p.isConfigProto(tt.protoPath); got != tt.want {
				t.Errorf("Params{ConfigProto: %q}.isConfigProto(%q) = %v, want %v",
					tt.configProto, tt.protoPath, got, tt.want)
			}
		})
	}
}

func checkNoInternalLeak(t *testing.T, msg string) {
	t.Helper()
	sensitive := []string{".go:", "goroutine ", "runtime.", "panic:", "/Users/", "/home/", "github.com/"}
	for _, s := range sensitive {
		if strings.Contains(msg, s) {
			t.Errorf("error message leaks internal detail %q: %s", s, msg)
		}
	}
}

// TestParamsSet_EdgeCases probes Params.Set with various inputs.
func TestParamsSet_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("unknown_parameter", func(t *testing.T) {
		t.Parallel()
		p := &Params{}
		err := p.Set("evil_param", "value")
		if err == nil {
			t.Fatal("expected error for unknown parameter")
		}
		checkNoInternalLeak(t, err.Error())
	})

	t.Run("empty_name", func(t *testing.T) {
		t.Parallel()
		p := &Params{}
		err := p.Set("", "value")
		if err == nil {
			t.Fatal("expected error for empty parameter name")
		}
	})

	t.Run("config_proto_with_path_traversal", func(t *testing.T) {
		t.Parallel()
		p := &Params{}
		err := p.Set("config_proto", "../../etc/passwd")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// The path is stored as-is. isConfigProto does exact match
		// when the pattern contains a slash.
		if !p.isConfigProto("../../etc/passwd") {
			t.Error("exact path match should work")
		}
		// Should NOT match other paths.
		if p.isConfigProto("etc/passwd") {
			t.Error("should not match partial path")
		}
	})
}
