package cli_test

import (
	"flag"
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
