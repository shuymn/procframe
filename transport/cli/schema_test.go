package cli_test

import (
	"encoding/json"
	"testing"

	"github.com/shuymn/procframe/transport/cli"
)

func TestCommandInfo_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	info := cli.CommandInfo{
		Command:   "echo run",
		Summary:   "Echo a message",
		Procedure: "/test.v1.EchoService/Echo",
		Flags: []cli.SchemaField{
			{Name: "message", Type: "string"},
			{Name: "count", Type: "int32"},
			{Name: "state", Type: "enum", EnumValues: []string{"open", "closed"}},
			{Name: "tags", Type: "string", Repeated: true},
		},
		Output: []cli.SchemaField{
			{Name: "message", Type: "string"},
		},
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got cli.CommandInfo
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Command != info.Command {
		t.Fatalf("want command=%q, got %q", info.Command, got.Command)
	}
	if got.Summary != info.Summary {
		t.Fatalf("want summary=%q, got %q", info.Summary, got.Summary)
	}
	if got.Procedure != info.Procedure {
		t.Fatalf("want procedure=%q, got %q", info.Procedure, got.Procedure)
	}
	if len(got.Flags) != 4 {
		t.Fatalf("want 4 flags, got %d", len(got.Flags))
	}
	if got.Flags[2].EnumValues[0] != "open" {
		t.Fatalf("want enum_values[0]=open, got %q", got.Flags[2].EnumValues[0])
	}
	if !got.Flags[3].Repeated {
		t.Fatal("want tags to be repeated")
	}
	if len(got.Output) != 1 {
		t.Fatalf("want 1 output field, got %d", len(got.Output))
	}
}

func TestSchemaField_EnumValuesOmittedWhenEmpty(t *testing.T) {
	t.Parallel()

	field := cli.SchemaField{Name: "message", Type: "string"}
	data, err := json.Marshal(field)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := raw["enum_values"]; ok {
		t.Fatal("want enum_values omitted when empty")
	}
	if _, ok := raw["repeated"]; ok {
		t.Fatal("want repeated omitted when false")
	}
}

func TestCommandInfo_StreamingOmittedWhenFalse(t *testing.T) {
	t.Parallel()

	info := cli.CommandInfo{
		Command:   "echo run",
		Procedure: "/test.v1.EchoService/Echo",
		Flags:     []cli.SchemaField{},
		Output:    []cli.SchemaField{},
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := raw["streaming"]; ok {
		t.Fatal("want streaming omitted when false")
	}
	if _, ok := raw["summary"]; ok {
		t.Fatal("want summary omitted when empty")
	}
}
