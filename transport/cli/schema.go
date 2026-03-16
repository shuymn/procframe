package cli

// CommandInfo describes a single CLI command's type information.
type CommandInfo struct {
	Command   string        `json:"command"`
	Summary   string        `json:"summary,omitempty"`
	Procedure string        `json:"procedure"`
	Flags     []SchemaField `json:"flags"`
	Output    []SchemaField `json:"output"`
	Streaming bool          `json:"streaming,omitempty"`
}

// SchemaField describes a single field within a message.
type SchemaField struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Repeated    bool     `json:"repeated,omitempty"`
	EnumValues  []string `json:"enum_values,omitempty"`
}
