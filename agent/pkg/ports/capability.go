package ports

// CapabilityInvocation is one MCP tools/call on a named server (planner "capability" id = server name).
type CapabilityInvocation struct {
	CapabilityName string
	Tool           string
	Arguments      map[string]any
}

// Capability is a server id and the tool names it exposes (routing / planner allow-list).
type Capability struct {
	CapabilityName string   `json:"capability_name"`
	Tools          []string `json:"tools"`
}
