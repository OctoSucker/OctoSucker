package ports

// CapabilityInvocation is one tool call on a named capability.
type CapabilityInvocation struct {
	CapabilityName string
	Tool           string
	Arguments      map[string]any
}

// Capability is a capability id and the tool names it exposes (routing / planner allow-list).
type Capability struct {
	CapabilityName string   `json:"capability_name"`
	Tools          []string `json:"tools"`
}
