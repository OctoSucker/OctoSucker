package capability

import (
	"context"
	"fmt"

	"github.com/OctoSucker/agent/pkg/mcpclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

// CapabilityRegistry wraps MCPRouter: snapshot of ListCapabilities at construction, plus the router for tool input schemas.
type CapabilityRegistry struct {
	mcp  *mcpclient.MCPRouter
	caps map[string]ports.Capability
}

// NewCapabilityRegistry loads capabilities once from the MCP router.
func NewCapabilityRegistry(ctx context.Context, mcp *mcpclient.MCPRouter) (*CapabilityRegistry, error) {
	if mcp == nil {
		return nil, fmt.Errorf("capability: nil MCPRouter")
	}
	m, err := mcp.ListCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]ports.Capability{}
	}
	return &CapabilityRegistry{mcp: mcp, caps: m}, nil
}

// AllCapabilities returns a snapshot for read-only routing/planner use.
func (r *CapabilityRegistry) AllCapabilities() map[string]ports.Capability {
	out := make(map[string]ports.Capability, len(r.caps))
	for k, v := range r.caps {
		out[k] = ports.Capability{ID: v.ID, Tools: append([]string(nil), v.Tools...)}
	}
	return out
}

// ToolInputSchemasByName maps MCP tool name → JSON input schema (for planner step arguments).
func (r *CapabilityRegistry) ToolInputSchemasByName() map[string]any {
	if r.mcp == nil {
		return map[string]any{}
	}
	out := make(map[string]any)
	for _, spec := range r.mcp.CachedToolSpecs() {
		out[spec.Name] = spec.InputSchema
	}
	return out
}

func (r *CapabilityRegistry) FirstTool(capID string) string {
	c, ok := r.caps[capID]
	if !ok || len(c.Tools) == 0 {
		return ""
	}
	return c.Tools[0]
}

func (r *CapabilityRegistry) Tools(capID string) []string {
	c, ok := r.caps[capID]
	if !ok {
		return nil
	}
	out := make([]string, len(c.Tools))
	copy(out, c.Tools)
	return out
}
