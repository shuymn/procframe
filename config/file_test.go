package config

import (
	"errors"
	"testing"
)

func TestRedactSecretErrorPreservesWrapping(t *testing.T) {
	t.Parallel()

	baseErr := errors.New(`invalid value "secret-token"`)
	err := redactSecretError(
		"parse config JSON field \"apiToken\"",
		baseErr,
		[]byte(`{"apiToken":"secret-token"}`),
		[]string{"apiToken"},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, baseErr) {
		t.Fatalf("want wrapped base error, got: %v", err)
	}
	if got := err.Error(); got == baseErr.Error() {
		t.Fatalf("want redacted message, got original: %q", got)
	}
	if got := err.Error(); got == "" {
		t.Fatal("want non-empty error message")
	}
	if got := err.Error(); got != `parse config JSON field "apiToken": invalid value `+RedactedPlaceholder {
		t.Fatalf("unexpected error message: %q", got)
	}
}
