package toolcontract

// ToolDescriptor is the planner-facing, provider-independent tool contract.
type ToolDescriptor struct {
	Provider     string         `json:"provider"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Capabilities []string       `json:"capabilities,omitempty"`
	Risk         string         `json:"risk,omitempty"`
	OutputTrust  string         `json:"output_trust,omitempty"`
	InputSchema  map[string]any `json:"input_schema"`
}
