package codegen

import (
	"strings"
	"testing"
)

func TestFieldToFlagName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"log_level", "log-level"},
		{"message", "message"},
		{"org_name", "org-name"},
		{"max_retry_count", "max-retry-count"},
		{"id", "id"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := fieldToFlagName(tt.input)
			if got != tt.want {
				t.Fatalf("fieldToFlagName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEnumCLIValues(t *testing.T) {
	t.Parallel()

	t.Run("strips prefix and lowercases, excludes zero value", func(t *testing.T) {
		t.Parallel()
		values := []enumValueInfo{
			{ProtoName: "PULL_REQUEST_STATE_UNSPECIFIED", Number: 0},
			{ProtoName: "PULL_REQUEST_STATE_OPEN", Number: 1},
			{ProtoName: "PULL_REQUEST_STATE_CLOSED", Number: 2},
		}
		got, err := enumCLIValues("PullRequestState", values)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("want 2 mappings (zero value excluded), got %d", len(got))
		}
		if got[0].CLIValue != "open" || got[0].Number != 1 {
			t.Fatalf("want open/1, got %s/%d", got[0].CLIValue, got[0].Number)
		}
		if got[1].CLIValue != "closed" || got[1].Number != 2 {
			t.Fatalf("want closed/2, got %s/%d", got[1].CLIValue, got[1].Number)
		}
	})

	t.Run("detects collision", func(t *testing.T) {
		t.Parallel()
		// FOO_A and FOO_BAR_A both strip to "a" after prefix "FOO_BAR_"
		// Actually let's make a real collision: same stripped values
		values := []enumValueInfo{
			{ProtoName: "STATUS_UNSPECIFIED", Number: 0},
			{ProtoName: "STATUS_OK", Number: 1},
			{ProtoName: "STATUS_OK", Number: 2},
		}
		_, err := enumCLIValues("Status", values)
		if err == nil {
			t.Fatal("expected error for collision")
		}
	})

	t.Run("short enum name", func(t *testing.T) {
		t.Parallel()
		values := []enumValueInfo{
			{ProtoName: "X_UNSPECIFIED", Number: 0},
			{ProtoName: "X_A", Number: 1},
			{ProtoName: "X_B", Number: 2},
		}
		got, err := enumCLIValues("X", values)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("want 2 mappings, got %d", len(got))
		}
		if got[0].CLIValue != "a" || got[1].CLIValue != "b" {
			t.Fatalf("want a/b, got %s/%s", got[0].CLIValue, got[1].CLIValue)
		}
	})

	t.Run("no common prefix falls back to lowercase", func(t *testing.T) {
		t.Parallel()
		values := []enumValueInfo{
			{ProtoName: "COLOR_UNSPECIFIED", Number: 0},
			{ProtoName: "ALPHA", Number: 1},
			{ProtoName: "BETA", Number: 2},
		}
		got, err := enumCLIValues("Color", values)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got[0].CLIValue != "alpha" || got[1].CLIValue != "beta" {
			t.Fatalf("want alpha/beta, got %s/%s", got[0].CLIValue, got[1].CLIValue)
		}
	})
}

func TestCamelToUpperSnake(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"PullRequestState", "PULL_REQUEST_STATE"},
		{"State", "STATE"},
		{"HTTPStatus", "HTTP_STATUS"},
		{"X", "X"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := camelToUpperSnake(tt.input)
			if got != tt.want {
				t.Fatalf("camelToUpperSnake(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestCamelToUpperSnake_EdgeCases probes camelToUpperSnake
// with boundary inputs that existing tests don't cover.
func TestCamelToUpperSnake_EdgeCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty_string", "", ""},
		{"all_uppercase", "ALLCAPS", "ALLCAPS"},
		{"single_lowercase", "x", "X"},
		{"numbers_in_name", "HTTP2Status", "HTTP2STATUS"},
		{"trailing_uppercase_run", "getHTTP", "GET_HTTP"},
		{"underscore_in_input", "already_snake", "ALREADY_SNAKE"},
		{"long_input", strings.Repeat("Abc", 100), "ABC" + strings.Repeat("_ABC", 99)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := camelToUpperSnake(tc.input)
			if got != tc.want {
				t.Errorf("camelToUpperSnake(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestEnumCLIValues_EdgeCases probes enumCLIValues with
// adversarial inputs.
func TestEnumCLIValues_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil_entry_in_slice", func(t *testing.T) {
		t.Parallel()
		values := []enumValueInfo{
			{ProtoName: "STATUS_OK", Number: 1},
			{ProtoName: "STATUS_ERROR", Number: 2},
		}
		got, err := enumCLIValues("Status", values)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("want 2 mappings, got %d", len(got))
		}
	})

	t.Run("empty_type_name", func(t *testing.T) {
		t.Parallel()
		values := []enumValueInfo{
			{ProtoName: "VALUE_A", Number: 1},
			{ProtoName: "VALUE_B", Number: 2},
		}
		// Empty type name -> prefix is "_" -> stripping won't remove prefix.
		_, err := enumCLIValues("", values)
		if err != nil {
			t.Fatalf("unexpected error for empty type name: %v", err)
		}
	})

	t.Run("all_zero_values", func(t *testing.T) {
		t.Parallel()
		values := []enumValueInfo{
			{ProtoName: "UNSPECIFIED", Number: 0},
		}
		got, err := enumCLIValues("Status", values)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("want 0 mappings (all zero values), got %d", len(got))
		}
	})

	t.Run("case_folding_collision", func(t *testing.T) {
		t.Parallel()
		// After prefix strip and lowercase: both become "ok"
		values := []enumValueInfo{
			{ProtoName: "STATUS_OK", Number: 1},
			{ProtoName: "STATUS_Ok", Number: 2}, // different case, same after lowercase
		}
		_, err := enumCLIValues("Status", values)
		if err == nil {
			t.Fatal("expected collision error for case-folded values")
		}
		checkNoInternalLeak(t, err.Error())
	})
}

// TestFieldToFlagName_SpecialChars verifies that flag name
// generation handles proto field names with unexpected characters.
func TestFieldToFlagName_SpecialChars(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"no_underscores", "simple", "simple"},
		{"double_underscore", "a__b", "a--b"},
		{"leading_underscore", "_hidden", "-hidden"},
		{"trailing_underscore", "field_", "field-"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := fieldToFlagName(tc.input)
			if got != tc.want {
				t.Errorf("fieldToFlagName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
