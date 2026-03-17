package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/shuymn/procframe"
)

func TestOutputFormatFromContext_Default(t *testing.T) {
	t.Parallel()

	got := OutputFormatFromContext(t.Context())
	if got != outputText {
		t.Fatalf("want text output, got %q", got)
	}
}

func TestOutputFormatFromContext_Set(t *testing.T) {
	t.Parallel()

	ctx := WithOutputFormat(t.Context(), OutputJSON)
	got := OutputFormatFromContext(ctx)
	if got != OutputJSON {
		t.Fatalf("want OutputJSON, got %q", got)
	}
}

func TestJSONPayloadFromContext_NotSet(t *testing.T) {
	t.Parallel()

	_, ok := JSONPayloadFromContext(t.Context())
	if ok {
		t.Fatal("want ok=false when not set")
	}
}

func TestJSONPayloadFromContext_Set(t *testing.T) {
	t.Parallel()

	ctx := WithJSONPayload(t.Context(), `{"message":"hi"}`)
	payload, ok := JSONPayloadFromContext(ctx)
	if !ok {
		t.Fatal("want ok=true")
	}
	if payload != `{"message":"hi"}` {
		t.Fatalf("want payload, got %q", payload)
	}
}

func TestFormatErrorJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := formatErrorJSON(&buf, &procframe.Status{
		Code:    procframe.CodeNotFound,
		Message: "resource not found",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if got.Error.Code != "not_found" {
		t.Fatalf("want code=not_found, got %q", got.Error.Code)
	}
	if got.Error.Message != "resource not found" {
		t.Fatalf("want message, got %q", got.Error.Message)
	}
	if got.Error.Retryable {
		t.Fatal("want retryable=false")
	}
}

func TestFormatErrorJSON_Retryable(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := formatErrorJSON(
		&buf,
		&procframe.Status{
			Code:      procframe.CodeUnavailable,
			Message:   "service unavailable",
			Retryable: true,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		Error struct {
			Retryable bool `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !got.Error.Retryable {
		t.Fatal("want retryable=true")
	}
}

func TestFormatErrorJSON_BrokenWriter(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("formatErrorJSON panicked: %v", r)
		}
	}()

	err := formatErrorJSON(&brokenWriter{}, &procframe.Status{
		Code:    procframe.CodeInternal,
		Message: "test error",
	})
	if err == nil {
		t.Fatal("expected error for broken writer")
	}
}

type brokenWriter struct{}

func (w *brokenWriter) Write(_ []byte) (int, error) {
	return 0, fmt.Errorf("broken writer")
}
