package config

import (
	"encoding/json"
	"errors"
	"strings"
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

func TestDecodeJSONOverlayValue_NullSkipsParser(t *testing.T) {
	t.Parallel()

	called := false
	got, err := decodeJSONOverlayValue("apiToken", json.RawMessage(" null "), map[string]JSONFieldParser{
		"apiToken": func(_ json.RawMessage) (any, error) {
			called = true
			return "parsed", nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("parser should not be called for null")
	}
	if got != nil {
		t.Fatalf("want nil for null value, got %v", got)
	}
}

func TestNormalizeJSONFieldParsers_RejectsCanonicalCollisions(t *testing.T) {
	t.Parallel()

	_, err := normalizeJSONFieldParsers(map[string]JSONFieldParser{
		"apiToken": func(_ json.RawMessage) (any, error) {
			return nil, nil
		},
		"api_token": func(_ json.RawMessage) (any, error) {
			return nil, nil
		},
	}, map[string]string{
		"apiToken":  "apiToken",
		"api_token": "apiToken",
	})
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(err.Error(), `"apiToken"`) || !strings.Contains(err.Error(), `"api_token"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
