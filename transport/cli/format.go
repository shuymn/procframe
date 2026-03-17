package cli

import (
	"context"
	"encoding/json"
	"io"

	"github.com/shuymn/procframe"
)

// OutputFormat controls how a leaf command formats its output.
type OutputFormat string

const (
	// outputText is the default human-readable format (multiline JSON).
	outputText OutputFormat = "text"
	// OutputJSON is compact JSON for agent consumption.
	OutputJSON OutputFormat = "json"
)

type contextKey int

const (
	keyOutputFormat contextKey = iota
	keyJSONPayload
)

// WithOutputFormat returns a child context carrying the given output format.
func WithOutputFormat(ctx context.Context, format OutputFormat) context.Context {
	return context.WithValue(ctx, keyOutputFormat, format)
}

// OutputFormatFromContext extracts the output format from the context.
// Returns the default text format if not set.
func OutputFormatFromContext(ctx context.Context) OutputFormat {
	if v, ok := ctx.Value(keyOutputFormat).(OutputFormat); ok {
		return v
	}
	return outputText
}

// WithJSONPayload returns a child context carrying the raw JSON payload.
func WithJSONPayload(ctx context.Context, payload string) context.Context {
	return context.WithValue(ctx, keyJSONPayload, payload)
}

// JSONPayloadFromContext extracts the raw JSON payload from the context.
// Returns ("", false) if not set.
func JSONPayloadFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyJSONPayload).(string)
	return v, ok
}

// structuredError is the JSON envelope for error output.
type structuredError struct {
	Error structuredErrorBody `json:"error"`
}

type structuredErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// formatErrorJSON writes a structured error to w as a single JSON line.
func formatErrorJSON(w io.Writer, status *procframe.Status) error {
	se := structuredError{
		Error: structuredErrorBody{
			Code:      string(status.Code),
			Message:   status.Message,
			Retryable: status.Retryable,
		},
	}
	data, merr := json.Marshal(se)
	if merr != nil {
		return merr
	}
	if _, werr := w.Write(data); werr != nil {
		return werr
	}
	_, werr := w.Write([]byte("\n"))
	return werr
}
