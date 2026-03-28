package capability

import (
	"context"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CapabilityRunner interface {
	Name() string
	HasTool(tool string) bool
	Tool(tool string) (*mcp.Tool, error)
	ToolList(ctx context.Context) ([]*mcp.Tool, error)
	Invoke(ctx context.Context, inv ports.CapabilityInvocation) (ports.ToolResult, error)
}
