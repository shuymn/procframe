package codegen

import "testing"

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
