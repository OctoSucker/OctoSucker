package capability

import (
	"context"
	"fmt"

	"github.com/OctoSucker/agent/pkg/mcpclient"
)

// CapabilityRegistry wraps MCPRouter: snapshot of ListCapabilities at construction, plus the router for tool input schemas.
type CapabilityRegistry struct {
	mcp  *mcpclient.MCPRouter
	caps map[string]mcpclient.Capability
}

// NewCapabilityRegistry loads capabilities once from the MCP router.
func NewCapabilityRegistry(ctx context.Context, mcp *mcpclient.MCPRouter) (*CapabilityRegistry, error) {
	if mcp == nil {
		return nil, fmt.Errorf("capability: nil MCPRouter")
	}
	toolsByServer, err := mcp.ListCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	caps := make(map[string]mcpclient.Capability, len(toolsByServer))
	for server, tools := range toolsByServer {
		toolNames := make([]string, 0, len(tools))
		for _, t := range tools {
			if t.Name == "" {
				continue
			}
			toolNames = append(toolNames, t.Name)
		}
		caps[server] = mcpclient.Capability{CapabilityName: server, Tools: toolNames}
	}

	return &CapabilityRegistry{mcp: mcp, caps: caps}, nil
}

// AllCapabilities returns a snapshot for read-only routing/planner use.
func (r *CapabilityRegistry) AllCapabilities() map[string]mcpclient.Capability {
	out := make(map[string]mcpclient.Capability, len(r.caps))
	for k, v := range r.caps {
		out[k] = mcpclient.Capability{CapabilityName: v.CapabilityName, Tools: append([]string(nil), v.Tools...)}
	}
	return out
}

// ToolInputSchemasByName maps MCP tool name → JSON input schema (for planner step arguments).
func (r *CapabilityRegistry) ToolInputSchemasByName() map[string]any {
	if r.mcp == nil {
		return map[string]any{}
	}
	out := make(map[string]any)
	for _, spec := range r.mcp.ListToolSpecs() {
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
