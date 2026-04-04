package tools

import (
	"context"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolProvider is a named tool source (builtin exec, skills, one MCP server, …) exposing MCP-shaped tools.
type ToolProvider interface {
	Name() string
	HasTool(tool string) bool
	Tool(tool string) (*mcp.Tool, error)
	ToolList(ctx context.Context) ([]*mcp.Tool, error)
	Invoke(ctx context.Context, localTool string, arguments map[string]any) (ports.ToolResult, error)
}
