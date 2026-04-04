package tools

import (
	"context"
	"fmt"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// ListRegisteredToolsTool returns all tool names the planner may use.
	ListRegisteredToolsTool = "list_registered_tools"
	// PlannerToolAppendixTool returns the full planner tools appendix text.
	PlannerToolAppendixTool = "get_planner_tool_appendix"
)

type introspectionBackend struct {
	reg *ToolRegistry
}

func newIntrospectionBackend(reg *ToolRegistry) *introspectionBackend {
	return &introspectionBackend{reg: reg}
}

// Name is the ToolRegistry.Backends map key for this provider (not a user-facing tool id).
func (introspectionBackend) Name() string { return "registry" }

func (r *introspectionBackend) HasTool(name string) bool {
	return name == ListRegisteredToolsTool || name == PlannerToolAppendixTool
}

func registryEmptyObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
}

func (r *introspectionBackend) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	return []*mcp.Tool{
		{
			Name:        ListRegisteredToolsTool,
			Description: "List all registered tool names (flat ids), same set the planner may call",
			InputSchema: registryEmptyObjectSchema(),
		},
		{
			Name:        PlannerToolAppendixTool,
			Description: "Return the full planner tools appendix (tool names + JSON Schema), matching what the planner sees",
			InputSchema: registryEmptyObjectSchema(),
		},
	}, nil
}

func (r *introspectionBackend) Tool(tool string) (*mcp.Tool, error) {
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

func (r *introspectionBackend) Invoke(ctx context.Context, localTool string, arguments map[string]any) (ports.ToolResult, error) {
	_ = arguments
	switch localTool {
	case ListRegisteredToolsTool:
		ids := r.reg.AllToolIDs()
		return ports.ToolResult{
			Output: map[string]any{
				"tools": ids,
			},
		}, nil
	case PlannerToolAppendixTool:
		return ports.ToolResult{
			Output: map[string]any{
				"appendix": r.reg.PlannerToolAppendix(),
			},
		}, nil
	default:
		return ports.ToolResult{Err: fmt.Errorf("registry: unknown tool %q", localTool)}, fmt.Errorf("registry: unknown tool %q", localTool)
	}
}
