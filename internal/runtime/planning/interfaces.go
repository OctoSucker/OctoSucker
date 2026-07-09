package planning

import (
	"context"

	"github.com/OctoSucker/octosucker/internal/runtime/toolrouting"
	tools "github.com/OctoSucker/octosucker/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ToolCatalog interface {
	Tool(name string) (*mcp.Tool, error)
	AllToolIDs() []string
	ProviderDescriptors() []tools.ProviderDescriptor
	SkillDescriptors() []map[string]any
}

type RouteGraph interface {
	Confidence(ctx context.Context, intent string, last toolrouting.Node) float64
	Frontier(ctx context.Context, intent string, last *toolrouting.Node, exclude *toolrouting.Node) []toolrouting.Node
}
