package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// MergeJSONFile overlays top-level JSON fields from path onto dst.
// This enables defaults -> file merge without dropping pre-populated defaults.
func MergeJSONFile(path string, dst proto.Message, secretFields ...string) (map[string]struct{}, error) {
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

	overlay, presentFields, err := parseJSONOverlay(data, configJSONFieldNames(dst))
	if err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}
	for k, v := range overlay {
		base[k] = v
	}

	merged, err := json.Marshal(base)
	if err != nil {
		return nil, fmt.Errorf("marshal merged config JSON: %w", err)
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(merged, dst); err != nil {
		return nil, redactSecretJSONError(err, merged, secretFields)
	}
	return presentFields, nil
}

func redactSecretJSONError(err error, merged []byte, secretFields []string) error {
	redactedErr := err.Error()
	for _, token := range secretJSONValueTokens(merged, secretFields) {
		redactedErr = strings.ReplaceAll(redactedErr, token, RedactedPlaceholder)
	}
	return fmt.Errorf("unmarshal merged config JSON: %v", redactedErr)
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
) (map[string]any, map[string]struct{}, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}

	overlay := make(map[string]any, len(raw))
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

		var decoded any
		if err := json.Unmarshal(value, &decoded); err != nil {
			return nil, nil, err
		}
		overlay[canonicalField] = decoded
		if string(value) == "null" {
			continue
		}
		presentFields[canonicalField] = struct{}{}
	}
	return overlay, presentFields, nil
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
