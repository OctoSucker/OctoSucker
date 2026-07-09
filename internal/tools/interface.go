package tools

import (
	"context"

	types "github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Provider is a named tool source (builtin exec, skills, one MCP server, …) exposing MCP-shaped tools.
type Provider interface {
	// Name returns the stable tool-provider name (Registry.providersByName key) and a short human description.
	Name() (id string, description string)
	HasTool(tool string) bool
	Tool(tool string) (*mcp.Tool, error)
	ToolList(ctx context.Context) ([]*mcp.Tool, error)
	Invoke(ctx context.Context, localTool string, arguments map[string]any) (types.ToolResult, error)
}

// ClosableProvider is a Provider that holds external resources and must be
// explicitly shut down during runtime teardown.
type ClosableProvider interface {
	Provider
	Close() error
}
