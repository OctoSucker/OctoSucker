package ports

// ToolInvocation is one tool call: Tool is the globally unique flat tool name.
type ToolInvocation struct {
	Tool      string
	Arguments map[string]any
}
