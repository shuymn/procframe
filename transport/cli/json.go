package cli

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// UnmarshalJSONField unmarshals rawJSON into the named field of msg
// without resetting existing fields. Used by generated code to set
// complex fields from JSON string flags.
// jsonFieldName must be a trusted protobuf JSON field name (not user input).
func UnmarshalJSONField(msg proto.Message, jsonFieldName, rawJSON string) error {
	// Validate that rawJSON is a single JSON value to prevent sibling field
	// injection via crafted input like: {"org":"x"},"limit":999
	if !json.Valid([]byte(rawJSON)) {
		return fmt.Errorf("invalid JSON value for field %s", jsonFieldName)
	}
	tmp := msg.ProtoReflect().New().Interface()
	wrapped := `{"` + jsonFieldName + `":` + rawJSON + `}`
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal([]byte(wrapped), tmp); err != nil {
		return err
	}
	proto.Merge(msg, tmp)
	return nil
}
