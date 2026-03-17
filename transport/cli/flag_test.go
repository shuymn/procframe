package cli_test

import (
	"flag"
	"fmt"
	"math"
	"testing"

	"github.com/shuymn/procframe/transport/cli"
)

func TestInt32Value(t *testing.T) {
	t.Parallel()

	t.Run("set and get", func(t *testing.T) {
		t.Parallel()
		var v int32
		fv := cli.NewInt32Value(&v)
		if err := fv.Set("42"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != 42 {
			t.Fatalf("want 42, got %d", v)
		}
		if s := fv.String(); s != "42" {
			t.Fatalf("want %q, got %q", "42", s)
		}
	})

	t.Run("negative", func(t *testing.T) {
		t.Parallel()
		var v int32
		fv := cli.NewInt32Value(&v)
		if err := fv.Set("-10"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != -10 {
			t.Fatalf("want -10, got %d", v)
		}
	})

	t.Run("overflow", func(t *testing.T) {
		t.Parallel()
		var v int32
		fv := cli.NewInt32Value(&v)
		if err := fv.Set("2147483648"); err == nil {
			t.Fatal("expected error for int32 overflow")
		}
	})

	t.Run("underflow", func(t *testing.T) {
		t.Parallel()
		var v int32
		fv := cli.NewInt32Value(&v)
		if err := fv.Set("-2147483649"); err == nil { // MinInt32 - 1
			t.Fatal("expected underflow error for int32")
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		var v int32
		fv := cli.NewInt32Value(&v)
		if err := fv.Set(""); err == nil {
			t.Fatal("expected error for empty int32")
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()
		var v int32
		fv := cli.NewInt32Value(&v)
		if err := fv.Set("abc"); err == nil {
			t.Fatal("expected error for non-numeric input")
		}
	})

	t.Run("implements flag.Value", func(t *testing.T) {
		t.Parallel()
		var v int32
		var _ flag.Value = cli.NewInt32Value(&v)
	})
}

func TestInt64Value(t *testing.T) {
	t.Parallel()

	t.Run("set and get", func(t *testing.T) {
		t.Parallel()
		var v int64
		fv := cli.NewInt64Value(&v)
		if err := fv.Set("9223372036854775807"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != 9223372036854775807 {
			t.Fatalf("want max int64, got %d", v)
		}
		if s := fv.String(); s != "9223372036854775807" {
			t.Fatalf("want %q, got %q", "9223372036854775807", s)
		}
	})
}

func TestUint32Value(t *testing.T) {
	t.Parallel()

	t.Run("set and get", func(t *testing.T) {
		t.Parallel()
		var v uint32
		fv := cli.NewUint32Value(&v)
		if err := fv.Set("4294967295"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != 4294967295 {
			t.Fatalf("want max uint32, got %d", v)
		}
	})

	t.Run("negative rejected", func(t *testing.T) {
		t.Parallel()
		var v uint32
		fv := cli.NewUint32Value(&v)
		if err := fv.Set("-1"); err == nil {
			t.Fatal("expected error for negative uint32")
		}
	})

	t.Run("overflow", func(t *testing.T) {
		t.Parallel()
		var v uint32
		fv := cli.NewUint32Value(&v)
		if err := fv.Set("4294967296"); err == nil {
			t.Fatal("expected error for uint32 overflow")
		}
	})
}

func TestUint64Value(t *testing.T) {
	t.Parallel()

	t.Run("set and get", func(t *testing.T) {
		t.Parallel()
		var v uint64
		fv := cli.NewUint64Value(&v)
		if err := fv.Set("18446744073709551615"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != 18446744073709551615 {
			t.Fatalf("want max uint64, got %d", v)
		}
	})

	t.Run("negative rejected", func(t *testing.T) {
		t.Parallel()
		var v uint64
		fv := cli.NewUint64Value(&v)
		if err := fv.Set("-1"); err == nil {
			t.Fatal("expected error for negative uint64")
		}
	})
}

func TestFloat32Value(t *testing.T) {
	t.Parallel()

	t.Run("set and get", func(t *testing.T) {
		t.Parallel()
		var v float32
		fv := cli.NewFloat32Value(&v)
		if err := fv.Set("3.14"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v < 3.13 || v > 3.15 {
			t.Fatalf("want ~3.14, got %f", v)
		}
	})

	t.Run("overflow", func(t *testing.T) {
		t.Parallel()
		var v float32
		fv := cli.NewFloat32Value(&v)
		huge := fmt.Sprintf("%e", math.MaxFloat64)
		if err := fv.Set(huge); err == nil {
			t.Fatal("expected overflow error for float32")
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()
		var v float32
		fv := cli.NewFloat32Value(&v)
		if err := fv.Set("not-a-number"); err == nil {
			t.Fatal("expected error for non-numeric input")
		}
	})
}

func TestEnumValue(t *testing.T) {
	t.Parallel()

	mappings := []cli.EnumMapping{
		{CLIValue: "open", Number: 1},
		{CLIValue: "closed", Number: 2},
		{CLIValue: "merged", Number: 3},
	}

	t.Run("exact match", func(t *testing.T) {
		t.Parallel()
		var v int32
		fv := cli.NewEnumValue(&v, mappings, "State")
		if err := fv.Set("open"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != 1 {
			t.Fatalf("want 1, got %d", v)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		t.Parallel()
		var v int32
		fv := cli.NewEnumValue(&v, mappings, "State")
		if err := fv.Set("CLOSED"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != 2 {
			t.Fatalf("want 2, got %d", v)
		}
	})

	t.Run("mixed case", func(t *testing.T) {
		t.Parallel()
		var v int32
		fv := cli.NewEnumValue(&v, mappings, "State")
		if err := fv.Set("Merged"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != 3 {
			t.Fatalf("want 3, got %d", v)
		}
	})

	t.Run("invalid value", func(t *testing.T) {
		t.Parallel()
		var v int32
		fv := cli.NewEnumValue(&v, mappings, "State")
		if err := fv.Set("draft"); err == nil {
			t.Fatal("expected error for unknown enum value")
		}
	})

	t.Run("string shows current", func(t *testing.T) {
		t.Parallel()
		var v int32
		fv := cli.NewEnumValue(&v, mappings, "State")
		if s := fv.String(); s != "" {
			t.Fatalf("want empty string for zero value, got %q", s)
		}
		if err := fv.Set("open"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s := fv.String(); s != "open" {
			t.Fatalf("want %q, got %q", "open", s)
		}
	})
}

func TestBoolSliceValue(t *testing.T) {
	t.Parallel()

	t.Run("single value", func(t *testing.T) {
		t.Parallel()
		var v []bool
		fv := cli.NewBoolSliceValue(&v)
		if err := fv.Set("true"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v) != 1 || !v[0] {
			t.Fatalf("want [true], got %v", v)
		}
	})

	t.Run("accumulates", func(t *testing.T) {
		t.Parallel()
		var v []bool
		fv := cli.NewBoolSliceValue(&v)
		if err := fv.Set("true"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("false"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v) != 3 || !v[0] || v[1] || !v[2] {
			t.Fatalf("want [true false true], got %v", v)
		}
	})

	t.Run("string representation", func(t *testing.T) {
		t.Parallel()
		var v []bool
		fv := cli.NewBoolSliceValue(&v)
		if s := fv.String(); s != "[]" {
			t.Fatalf("want %q, got %q", "[]", s)
		}
		if err := fv.Set("true"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("false"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s := fv.String(); s != "[true,false]" {
			t.Fatalf("want %q, got %q", "[true,false]", s)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()
		var v []bool
		fv := cli.NewBoolSliceValue(&v)
		if err := fv.Set("notbool"); err == nil {
			t.Fatal("expected error for invalid bool")
		}
	})

	t.Run("implements flag.Value", func(t *testing.T) {
		t.Parallel()
		var v []bool
		var _ flag.Value = cli.NewBoolSliceValue(&v)
	})
}

func TestInt32SliceValue(t *testing.T) {
	t.Parallel()

	t.Run("accumulates", func(t *testing.T) {
		t.Parallel()
		var v []int32
		fv := cli.NewInt32SliceValue(&v)
		if err := fv.Set("1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("-2"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("3"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v) != 3 || v[0] != 1 || v[1] != -2 || v[2] != 3 {
			t.Fatalf("want [1 -2 3], got %v", v)
		}
	})

	t.Run("string representation", func(t *testing.T) {
		t.Parallel()
		var v []int32
		fv := cli.NewInt32SliceValue(&v)
		if s := fv.String(); s != "[]" {
			t.Fatalf("want %q, got %q", "[]", s)
		}
		if err := fv.Set("10"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("20"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s := fv.String(); s != "[10,20]" {
			t.Fatalf("want %q, got %q", "[10,20]", s)
		}
	})

	t.Run("overflow", func(t *testing.T) {
		t.Parallel()
		var v []int32
		fv := cli.NewInt32SliceValue(&v)
		if err := fv.Set("2147483648"); err == nil {
			t.Fatal("expected error for int32 overflow")
		}
	})

	t.Run("with flag.FlagSet", func(t *testing.T) {
		t.Parallel()
		var ids []int32
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.Var(cli.NewInt32SliceValue(&ids), "id", "add an id")
		err := fs.Parse([]string{"--id", "1", "--id", "2", "--id", "3"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
			t.Fatalf("want [1 2 3], got %v", ids)
		}
	})
}

func TestInt64SliceValue(t *testing.T) {
	t.Parallel()

	t.Run("accumulates", func(t *testing.T) {
		t.Parallel()
		var v []int64
		fv := cli.NewInt64SliceValue(&v)
		if err := fv.Set("9223372036854775807"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v) != 2 || v[0] != 9223372036854775807 || v[1] != -1 {
			t.Fatalf("unexpected values: %v", v)
		}
	})

	t.Run("string representation", func(t *testing.T) {
		t.Parallel()
		var v []int64
		fv := cli.NewInt64SliceValue(&v)
		if s := fv.String(); s != "[]" {
			t.Fatalf("want %q, got %q", "[]", s)
		}
	})
}

func TestUint32SliceValue(t *testing.T) {
	t.Parallel()

	t.Run("accumulates", func(t *testing.T) {
		t.Parallel()
		var v []uint32
		fv := cli.NewUint32SliceValue(&v)
		if err := fv.Set("100"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("200"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v) != 2 || v[0] != 100 || v[1] != 200 {
			t.Fatalf("unexpected values: %v", v)
		}
	})

	t.Run("negative rejected", func(t *testing.T) {
		t.Parallel()
		var v []uint32
		fv := cli.NewUint32SliceValue(&v)
		if err := fv.Set("-1"); err == nil {
			t.Fatal("expected error for negative uint32")
		}
	})

	t.Run("string representation", func(t *testing.T) {
		t.Parallel()
		var v []uint32
		fv := cli.NewUint32SliceValue(&v)
		if err := fv.Set("1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s := fv.String(); s != "[1]" {
			t.Fatalf("want %q, got %q", "[1]", s)
		}
	})
}

func TestUint64SliceValue(t *testing.T) {
	t.Parallel()

	t.Run("accumulates", func(t *testing.T) {
		t.Parallel()
		var v []uint64
		fv := cli.NewUint64SliceValue(&v)
		if err := fv.Set("18446744073709551615"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("0"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v) != 2 || v[0] != 18446744073709551615 || v[1] != 0 {
			t.Fatalf("unexpected values: %v", v)
		}
	})

	t.Run("string representation", func(t *testing.T) {
		t.Parallel()
		var v []uint64
		fv := cli.NewUint64SliceValue(&v)
		if s := fv.String(); s != "[]" {
			t.Fatalf("want %q, got %q", "[]", s)
		}
	})
}

func TestFloat32SliceValue(t *testing.T) {
	t.Parallel()

	t.Run("accumulates", func(t *testing.T) {
		t.Parallel()
		var v []float32
		fv := cli.NewFloat32SliceValue(&v)
		if err := fv.Set("1.5"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("2.5"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v) != 2 || v[0] != 1.5 || v[1] != 2.5 {
			t.Fatalf("unexpected values: %v", v)
		}
	})

	t.Run("overflow", func(t *testing.T) {
		t.Parallel()
		var v []float32
		fv := cli.NewFloat32SliceValue(&v)
		if err := fv.Set("3.5e+38"); err == nil {
			t.Fatal("expected error for float32 overflow")
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()
		var v []float32
		fv := cli.NewFloat32SliceValue(&v)
		if err := fv.Set("nan-string"); err == nil {
			t.Fatal("expected error for non-numeric input")
		}
	})

	t.Run("string representation", func(t *testing.T) {
		t.Parallel()
		var v []float32
		fv := cli.NewFloat32SliceValue(&v)
		if err := fv.Set("3.14"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s := fv.String(); s == "[]" {
			t.Fatalf("want non-empty, got %q", s)
		}
	})
}

func TestFloat64SliceValue(t *testing.T) {
	t.Parallel()

	t.Run("accumulates", func(t *testing.T) {
		t.Parallel()
		var v []float64
		fv := cli.NewFloat64SliceValue(&v)
		if err := fv.Set("1.1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("2.2"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("3.3"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v) != 3 || v[0] != 1.1 || v[1] != 2.2 || v[2] != 3.3 {
			t.Fatalf("unexpected values: %v", v)
		}
	})

	t.Run("string representation", func(t *testing.T) {
		t.Parallel()
		var v []float64
		fv := cli.NewFloat64SliceValue(&v)
		if s := fv.String(); s != "[]" {
			t.Fatalf("want %q, got %q", "[]", s)
		}
		if err := fv.Set("1.5"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s := fv.String(); s != "[1.5]" {
			t.Fatalf("want %q, got %q", "[1.5]", s)
		}
	})
}

func TestEnumSliceValue(t *testing.T) {
	t.Parallel()

	mappings := []cli.EnumMapping{
		{CLIValue: "open", Number: 1},
		{CLIValue: "closed", Number: 2},
		{CLIValue: "merged", Number: 3},
	}

	t.Run("accumulates", func(t *testing.T) {
		t.Parallel()
		var v []int32
		fv := cli.NewEnumSliceValue(&v, mappings, "State")
		if err := fv.Set("open"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("closed"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v) != 2 || v[0] != 1 || v[1] != 2 {
			t.Fatalf("want [1 2], got %v", v)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		t.Parallel()
		var v []int32
		fv := cli.NewEnumSliceValue(&v, mappings, "State")
		if err := fv.Set("MERGED"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v) != 1 || v[0] != 3 {
			t.Fatalf("want [3], got %v", v)
		}
	})

	t.Run("invalid value", func(t *testing.T) {
		t.Parallel()
		var v []int32
		fv := cli.NewEnumSliceValue(&v, mappings, "State")
		if err := fv.Set("draft"); err == nil {
			t.Fatal("expected error for unknown enum value")
		}
	})

	t.Run("string representation", func(t *testing.T) {
		t.Parallel()
		var v []int32
		fv := cli.NewEnumSliceValue(&v, mappings, "State")
		if s := fv.String(); s != "[]" {
			t.Fatalf("want %q, got %q", "[]", s)
		}
		if err := fv.Set("open"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("closed"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s := fv.String(); s != "[open,closed]" {
			t.Fatalf("want %q, got %q", "[open,closed]", s)
		}
	})

	t.Run("with flag.FlagSet", func(t *testing.T) {
		t.Parallel()
		var states []int32
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.Var(cli.NewEnumSliceValue(&states, mappings, "State"), "state", "filter state")
		err := fs.Parse([]string{"--state", "open", "--state", "merged"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(states) != 2 || states[0] != 1 || states[1] != 3 {
			t.Fatalf("want [1 3], got %v", states)
		}
	})
}

func TestStringSliceValue(t *testing.T) {
	t.Parallel()

	t.Run("single value", func(t *testing.T) {
		t.Parallel()
		var v []string
		fv := cli.NewStringSliceValue(&v)
		if err := fv.Set("foo"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v) != 1 || v[0] != "foo" {
			t.Fatalf("want [foo], got %v", v)
		}
	})

	t.Run("accumulates", func(t *testing.T) {
		t.Parallel()
		var v []string
		fv := cli.NewStringSliceValue(&v)
		if err := fv.Set("foo"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("bar"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("baz"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v) != 3 {
			t.Fatalf("want 3 elements, got %d", len(v))
		}
		if v[0] != "foo" || v[1] != "bar" || v[2] != "baz" {
			t.Fatalf("want [foo bar baz], got %v", v)
		}
	})

	t.Run("string representation", func(t *testing.T) {
		t.Parallel()
		var v []string
		fv := cli.NewStringSliceValue(&v)
		if s := fv.String(); s != "[]" {
			t.Fatalf("want %q, got %q", "[]", s)
		}
		if err := fv.Set("a"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := fv.Set("b"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s := fv.String(); s != "[a,b]" {
			t.Fatalf("want %q, got %q", "[a,b]", s)
		}
	})

	t.Run("with flag.FlagSet", func(t *testing.T) {
		t.Parallel()
		var tags []string
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.Var(cli.NewStringSliceValue(&tags), "tag", "add a tag")
		err := fs.Parse([]string{"--tag", "foo", "--tag", "bar"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tags) != 2 || tags[0] != "foo" || tags[1] != "bar" {
			t.Fatalf("want [foo bar], got %v", tags)
		}
	})
}

func TestFlagValue_NilPointerString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fn   func() string
	}{
		{"Int32Value", func() string { return (&cli.Int32Value{}).String() }},
		{"Int64Value", func() string { return (&cli.Int64Value{}).String() }},
		{"Uint32Value", func() string { return (&cli.Uint32Value{}).String() }},
		{"Uint64Value", func() string { return (&cli.Uint64Value{}).String() }},
		{"Float32Value", func() string { return (&cli.Float32Value{}).String() }},
		{"EnumValue", func() string { return (&cli.EnumValue{}).String() }},
		{"StringSliceValue", func() string { return (&cli.StringSliceValue{}).String() }},
		{"BoolSliceValue", func() string { return (&cli.BoolSliceValue{}).String() }},
		{"Int32SliceValue", func() string { return (&cli.Int32SliceValue{}).String() }},
		{"Int64SliceValue", func() string { return (&cli.Int64SliceValue{}).String() }},
		{"Uint32SliceValue", func() string { return (&cli.Uint32SliceValue{}).String() }},
		{"Uint64SliceValue", func() string { return (&cli.Uint64SliceValue{}).String() }},
		{"Float32SliceValue", func() string { return (&cli.Float32SliceValue{}).String() }},
		{"Float64SliceValue", func() string { return (&cli.Float64SliceValue{}).String() }},
		{"EnumSliceValue", func() string { return (&cli.EnumSliceValue{}).String() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s.String() panicked with nil ptr: %v", tt.name, r)
				}
			}()
			_ = tt.fn()
		})
	}
}
