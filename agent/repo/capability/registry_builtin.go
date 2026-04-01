package capability

import (
	"context"
	"fmt"
	"sort"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// RegistryCapabilityName is the built-in capability that exposes registry introspection tools.
	RegistryCapabilityName = "capability_registry"
	// ListCapabilitiesTool lists all registered capabilities and tool names (planner allow-list).
	ListCapabilitiesTool = "list_capabilities"
	// PlannerToolAppendixTool returns the full planner tools appendix (capability + tool + JSON Schema).
	PlannerToolAppendixTool = "get_planner_tool_appendix"
)

type registryBuiltinRunner struct {
	reg *CapabilityRegistry
}

func newRegistryBuiltinRunner(reg *CapabilityRegistry) *registryBuiltinRunner {
	return &registryBuiltinRunner{reg: reg}
}

func (registryBuiltinRunner) Name() string { return RegistryCapabilityName }

func (r *registryBuiltinRunner) HasTool(name string) bool {
	return name == ListCapabilitiesTool || name == PlannerToolAppendixTool
}

func registryEmptyObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
}

func (r *registryBuiltinRunner) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	return []*mcp.Tool{
		{
			Name:        ListCapabilitiesTool,
			Description: "List all registered capabilities and their tool names (same as planner allow-list)",
			InputSchema: registryEmptyObjectSchema(),
		},
		{
			Name:        PlannerToolAppendixTool,
			Description: "Return the full planner tools appendix text (capability + tool + JSON Schema), matching what the planner sees",
			InputSchema: registryEmptyObjectSchema(),
		},
	}, nil
}

func (r *registryBuiltinRunner) Tool(tool string) (*mcp.Tool, error) {
	tools, err := r.ToolList(context.Background())
	if err != nil {
		return nil, err
	}
	for _, t := range tools {
		if t != nil && t.Name == tool {
			return t, nil
		}
	}
	return nil, fmt.Errorf("registry: unknown tool %q", tool)
}

func (r *registryBuiltinRunner) Invoke(ctx context.Context, inv ports.CapabilityInvocation) (ports.ToolResult, error) {
	switch inv.Tool {
	case ListCapabilitiesTool:
		all := r.reg.AllCapabilities()
		names := make([]string, 0, len(all))
		for k := range all {
			names = append(names, k)
		}
		sort.Strings(names)
		list := make([]map[string]any, 0, len(names))
		for _, name := range names {
			c := all[name]
			tools := append([]string(nil), c.Tools...)
			sort.Strings(tools)
			list = append(list, map[string]any{
				"capability_name": name,
				"tools":           tools,
			})
		}
		return ports.ToolResult{
			Output: map[string]any{
				"capabilities": list,
			},
		}, nil
	case PlannerToolAppendixTool:
		return ports.ToolResult{
			Output: map[string]any{
				"appendix": r.reg.PlannerToolAppendix(),
			},
		}, nil
	default:
		return ports.ToolResult{Err: fmt.Errorf("registry: unknown tool %q", inv.Tool)}, fmt.Errorf("registry: unknown tool %q", inv.Tool)
	}
}
