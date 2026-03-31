package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// JSONFieldParser rewrites a top-level config JSON field before protojson
// unmarshaling. The returned value must be JSON-marshalable.
type JSONFieldParser func(raw json.RawMessage) (any, error)

var jsonNull = []byte("null")

// MergeJSONFile overlays top-level JSON fields from path onto dst.
// This enables defaults -> file merge without dropping pre-populated defaults.
func MergeJSONFile(path string, dst proto.Message, secretFields ...string) (map[string]struct{}, error) {
	return mergeJSONFile(path, dst, nil, secretFields...)
}

// MergeJSONFileWithParsers overlays top-level JSON fields from path onto dst.
// Parsers can normalize selected fields before protojson unmarshaling.
func MergeJSONFileWithParsers(
	path string,
	dst proto.Message,
	parsers map[string]JSONFieldParser,
	secretFields ...string,
) (map[string]struct{}, error) {
	return mergeJSONFile(path, dst, parsers, secretFields...)
}

func mergeJSONFile(
	path string,
	dst proto.Message,
	parsers map[string]JSONFieldParser,
	secretFields ...string,
) (map[string]struct{}, error) {
	if dst == nil {
		return nil, fmt.Errorf("destination message is nil")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}

	base, err := currentJSONMap(dst)
	if err != nil {
		return nil, err
	}

	fieldNames := configJSONFieldNames(dst)
	rawOverlay, presentFields, err := parseJSONOverlay(data, fieldNames)
	if err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}
	normalizedParsers := normalizeJSONFieldParsers(parsers, fieldNames)
	for k, raw := range rawOverlay {
		v, decodeErr := decodeJSONOverlayValue(k, raw, normalizedParsers)
		if decodeErr != nil {
			redactionData := data
			if canonicalData, marshalErr := json.Marshal(rawOverlay); marshalErr == nil {
				redactionData = canonicalData
			}
			return nil, redactSecretError(
				"parse config JSON field "+strconv.Quote(k),
				decodeErr,
				redactionData,
				secretFields,
			)
		}
		base[k] = v
	}

	merged, err := json.Marshal(base)
	if err != nil {
		return nil, fmt.Errorf("marshal merged config JSON: %w", err)
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(merged, dst); err != nil {
		return nil, redactSecretError("unmarshal merged config JSON", err, merged, secretFields)
	}
	return presentFields, nil
}

func redactSecretError(prefix string, err error, data []byte, secretFields []string) error {
	redactedErr := err.Error()
	for _, token := range secretJSONValueTokens(data, secretFields) {
		redactedErr = strings.ReplaceAll(redactedErr, token, RedactedPlaceholder)
	}
	return &redactedError{
		msg: prefix + ": " + redactedErr,
		err: err,
	}
}

type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string {
	return e.msg
}

func (e *redactedError) Unwrap() error {
	return e.err
}

func secretJSONValueTokens(merged []byte, secretFields []string) []string {
	if len(secretFields) == 0 {
		return nil
	}

	var values map[string]json.RawMessage
	if err := json.Unmarshal(merged, &values); err != nil {
		return nil
	}

	tokens := make([]string, 0, len(secretFields))
	seen := make(map[string]struct{}, len(secretFields))
	for _, field := range secretFields {
		raw, ok := values[field]
		if !ok || len(raw) == 0 {
			continue
		}
		tokens = appendUniqueToken(tokens, seen, string(raw))
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err == nil {
			tokens = appendUniqueToken(tokens, seen, decoded)
		}
	}
	return tokens
}

func parseJSONOverlay(
	data []byte,
	fieldNames map[string]string,
) (map[string]json.RawMessage, map[string]struct{}, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}

	overlay := make(map[string]json.RawMessage, len(raw))
	presentFields := make(map[string]struct{}, len(raw))
	seenFields := make(map[string]string, len(raw))
	for field, value := range raw {
		canonicalField := canonicalJSONFieldName(field, fieldNames)
		if prev, dup := seenFields[canonicalField]; dup {
			return nil, nil, fmt.Errorf(
				"duplicate config field %q via %q and %q",
				canonicalField,
				prev,
				field,
			)
		}
		seenFields[canonicalField] = field

		overlay[canonicalField] = value
		if bytes.Equal(bytes.TrimSpace(value), jsonNull) {
			continue
		}
		presentFields[canonicalField] = struct{}{}
	}
	return overlay, presentFields, nil
}

func decodeJSONOverlayValue(
	field string,
	value json.RawMessage,
	parsers map[string]JSONFieldParser,
) (any, error) {
	if bytes.Equal(bytes.TrimSpace(value), jsonNull) {
		return nil, nil
	}
	if parser := parsers[field]; parser != nil {
		return parser(value)
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func normalizeJSONFieldParsers(
	parsers map[string]JSONFieldParser,
	fieldNames map[string]string,
) map[string]JSONFieldParser {
	if len(parsers) == 0 {
		return nil
	}

	normalized := make(map[string]JSONFieldParser, len(parsers))
	for field, parser := range parsers {
		normalized[canonicalJSONFieldName(field, fieldNames)] = parser
	}
	return normalized
}

func configJSONFieldNames(msg proto.Message) map[string]string {
	fields := msg.ProtoReflect().Descriptor().Fields()
	names := make(map[string]string, fields.Len()*2)
	for i := range fields.Len() {
		field := fields.Get(i)
		jsonName := field.JSONName()
		names[jsonName] = jsonName
		names[string(field.Name())] = jsonName
	}
	return names
}

func canonicalJSONFieldName(field string, fieldNames map[string]string) string {
	if canonical, ok := fieldNames[field]; ok {
		return canonical
	}
	return field
}

func appendUniqueToken(tokens []string, seen map[string]struct{}, token string) []string {
	if token == "" {
		return tokens
	}
	if _, dup := seen[token]; dup {
		return tokens
	}
	seen[token] = struct{}{}
	return append(tokens, token)
}

func currentJSONMap(msg proto.Message) (map[string]any, error) {
	raw, err := protojson.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal current config: %w", err)
	}
	out := make(map[string]any)
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse current config JSON: %w", err)
	}
	return out, nil
}
