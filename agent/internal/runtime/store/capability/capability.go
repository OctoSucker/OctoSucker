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
	toolsByServer, err := mcp.ListCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	caps := make(map[string]ports.Capability, len(toolsByServer))
	for server, tools := range toolsByServer {
		toolNames := make([]string, 0, len(tools))
		for _, t := range tools {
			if t.Name == "" {
				continue
			}
			toolNames = append(toolNames, t.Name)
		}
		caps[server] = ports.Capability{CapabilityName: server, Tools: toolNames}
	}

	return &CapabilityRegistry{mcp: mcp, caps: caps}, nil
}

// AllCapabilities returns a snapshot for read-only routing/planner use.
func (r *CapabilityRegistry) AllCapabilities() map[string]ports.Capability {
	out := make(map[string]ports.Capability, len(r.caps))
	for k, v := range r.caps {
		out[k] = ports.Capability{CapabilityName: v.CapabilityName, Tools: append([]string(nil), v.Tools...)}
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

func (r *CapabilityRegistry) Tools(capID string) ([]string, error) {
	c, ok := r.caps[capID]
	if !ok {
		return nil, fmt.Errorf("capability: capability %q not found", capID)
	}
	return c.Tools, nil
}

func (r *CapabilityRegistry) CheckStepTool(capID, tool string) bool {
	tools, err := r.Tools(capID)
	if err != nil {
		return false
	}
	for _, name := range tools {
		if name == tool {
			return true
		}
	}
	return false
}
