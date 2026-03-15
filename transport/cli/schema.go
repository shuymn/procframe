package cli

// SchemaInfo describes a single procedure's type information.
type SchemaInfo struct {
	Procedure string        `json:"procedure"`
	Request   SchemaMessage `json:"request"`
	Response  SchemaMessage `json:"response"`
	Streaming bool          `json:"streaming"`
}

// SchemaMessage describes a protobuf message type.
type SchemaMessage struct {
	FullName string        `json:"full_name"`
	Fields   []SchemaField `json:"fields"`
}

// SchemaField describes a single field within a message.
type SchemaField struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Repeated   bool     `json:"repeated"`
	EnumValues []string `json:"enum_values,omitempty"`
}
