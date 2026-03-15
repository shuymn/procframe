package cli_test

import (
	"encoding/json"
	"testing"

	"github.com/shuymn/procframe/transport/cli"
)

func TestSchemaInfo_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	info := cli.SchemaInfo{
		Procedure: "/test.v1.EchoService/Echo",
		Request: cli.SchemaMessage{
			FullName: "test.v1.EchoRequest",
			Fields: []cli.SchemaField{
				{Name: "message", Type: "string"},
				{Name: "count", Type: "int32"},
				{Name: "state", Type: "enum", EnumValues: []string{"open", "closed"}},
				{Name: "tags", Type: "string", Repeated: true},
			},
		},
		Response: cli.SchemaMessage{
			FullName: "test.v1.EchoResponse",
			Fields: []cli.SchemaField{
				{Name: "message", Type: "string"},
			},
		},
		Streaming: false,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got cli.SchemaInfo
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Procedure != info.Procedure {
		t.Fatalf("want procedure=%q, got %q", info.Procedure, got.Procedure)
	}
	if len(got.Request.Fields) != 4 {
		t.Fatalf("want 4 request fields, got %d", len(got.Request.Fields))
	}
	if got.Request.Fields[2].EnumValues[0] != "open" {
		t.Fatalf("want enum_values[0]=open, got %q", got.Request.Fields[2].EnumValues[0])
	}
	if !got.Request.Fields[3].Repeated {
		t.Fatal("want tags to be repeated")
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
}
