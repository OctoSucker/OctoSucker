package catalog

import (
	"context"
	"fmt"
	"sort"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	CapabilityName             = "catalog"
	ToolListCapabilities       = "list_capabilities"
	ToolGetPlannerToolAppendix = "get_planner_tool_appendix"
)

// RegistryView is the subset of capability.CapabilityRegistry needed for catalog tools (avoids import cycles).
type RegistryView interface {
	AllCapabilities() map[string]ports.Capability
	PlannerToolAppendix(ctx context.Context) string
}

type Runner struct {
	reg RegistryView
}

func NewRunner(reg RegistryView) (*Runner, error) {
	if reg == nil {
		return nil, fmt.Errorf("catalog builtin: registry is required")
	}
	return &Runner{reg: reg}, nil
}

func (r *Runner) Name() string { return CapabilityName }

func emptyObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
}

func (r *Runner) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	return []*mcp.Tool{
		{
			Name:        ToolListCapabilities,
			Description: "List all registered capabilities and their tool names (same as planner allow-list)",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name:        ToolGetPlannerToolAppendix,
			Description: "Return the full planner tools appendix text (capability + tool + JSON Schema), matching what the planner sees",
			InputSchema: emptyObjectSchema(),
		},
	}, nil
}

func (r *Runner) HasTool(name string) bool {
	return name == ToolListCapabilities || name == ToolGetPlannerToolAppendix
}

func (r *Runner) Tool(tool string) (*mcp.Tool, error) {
	tools, err := r.ToolList(context.Background())
	if err != nil {
		return nil, err
	}
	for _, t := range tools {
		if t != nil && t.Name == tool {
			return t, nil
		}
	}
	return nil, fmt.Errorf("catalog builtin: unknown tool %q", tool)
}

func (r *Runner) Invoke(ctx context.Context, inv ports.CapabilityInvocation) (ports.ToolResult, error) {
	switch inv.Tool {
	case "", ToolListCapabilities:
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
			OK: true,
			Output: map[string]any{
				"capabilities": list,
			},
		}, nil
	case ToolGetPlannerToolAppendix:
		return ports.ToolResult{
			OK: true,
			Output: map[string]any{
				"appendix": r.reg.PlannerToolAppendix(ctx),
			},
		}, nil
	default:
		return ports.ToolResult{}, fmt.Errorf("catalog builtin: unknown tool %q", inv.Tool)
	}
}
