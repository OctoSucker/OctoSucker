package toolprovider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/OctoSucker/octosucker/engine/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// ListToolProvidersTool returns each tool provider’s stable id and description.
	ListToolProvidersTool = "list_tool_providers"
	// ListToolsForProviderTool returns tool descriptors (name, description, input_schema) for one provider.
	ListToolsForProviderTool = "list_tools_for_provider"
)

type introspectionBackend struct {
	reg *Registry
}

func newIntrospectionBackend(reg *Registry) *introspectionBackend {
	return &introspectionBackend{reg: reg}
}

// Name is the stable tool-provider name (Registry.ProvidersMap key); not an MCP tool name.
func (introspectionBackend) Name() (string, string) {
	return "tool_registry", "Planner introspection: list tools and dump tool appendix."
}

func (r *introspectionBackend) HasTool(name string) bool {
	switch name {
	case ListToolProvidersTool, ListToolsForProviderTool:
		return true
	default:
		return false
	}
}

func registryEmptyObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
}

func registryListToolsForProviderSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"provider": map[string]any{
				"type":        "string",
				"description": `Tool provider id (the "id" field from list_tool_providers).`,
			},
		},
		"required":             []string{"provider"},
		"additionalProperties": false,
	}
}

func introspectionProviderArg(arguments map[string]any) (string, error) {
	if arguments == nil {
		return "", fmt.Errorf("tool_registry: arguments required")
	}
	v, ok := arguments["provider"]
	if !ok {
		return "", fmt.Errorf("tool_registry: missing required argument %q", "provider")
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("tool_registry: argument %q must be a string", "provider")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("tool_registry: argument %q must be non-empty", "provider")
	}
	return s, nil
}

func (r *introspectionBackend) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	return []*mcp.Tool{
		{
			Name:        ListToolProvidersTool,
			Description: "List tool providers (builtin + MCP): each entry has id and description of what that source provides.",
			InputSchema: registryEmptyObjectSchema(),
		},
		{
			Name:        ListToolsForProviderTool,
			Description: "List tool descriptors exposed by one provider: name, description, and input_schema.",
			InputSchema: registryListToolsForProviderSchema(),
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

func (r *introspectionBackend) Invoke(ctx context.Context, localTool string, arguments map[string]any) (types.ToolResult, error) {
	if localTool != ListToolsForProviderTool {
		_ = arguments
	}
	switch localTool {
	case ListToolProvidersTool:
		return types.ToolResult{
			Output: map[string]any{
				"providers": r.reg.ProviderDescriptors(),
			},
		}, nil
	case ListToolsForProviderTool:
		prov, err := introspectionProviderArg(arguments)
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		tools, err := r.reg.ToolDescriptorsForProvider(ctx, prov)
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		return types.ToolResult{
			Output: map[string]any{
				"provider": prov,
				"tools":    tools,
			},
		}, nil
	default:
		return types.ToolResult{Err: fmt.Errorf("registry: unknown tool %q", localTool)}, fmt.Errorf("registry: unknown tool %q", localTool)
	}
}

// ProviderDescriptors returns every registered provider’s id and description, sorted by id.
func (r *Registry) ProviderDescriptors() []ProviderDescriptor {
	ids := make([]string, 0, len(r.ProvidersMap))
	for k := range r.ProvidersMap {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	out := make([]ProviderDescriptor, 0, len(ids))
	for _, id := range ids {
		p := r.ProvidersMap[id]
		_, desc := p.Name()
		out = append(out, ProviderDescriptor{ID: id, Description: desc})
	}
	return out
}

// ProviderDescriptor is one registered tool source (builtin or MCP session): stable id and human description.
type ProviderDescriptor struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type ToolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// ToolDescriptorsForProvider returns sorted tool descriptors exposed by the named provider (ProvidersMap key).
func (r *Registry) ToolDescriptorsForProvider(ctx context.Context, providerName string) ([]ToolDescriptor, error) {
	pname := strings.TrimSpace(providerName)
	if pname == "" {
		return nil, fmt.Errorf("tool registry: provider name is required")
	}
	p, ok := r.ProvidersMap[pname]
	if !ok {
		return nil, fmt.Errorf("tool registry: unknown tool provider %q", pname)
	}
	tools, err := p.ToolList(ctx)
	if err != nil {
		return nil, fmt.Errorf("tool registry: list tools for provider %q: %w", pname, err)
	}
	out := make([]ToolDescriptor, 0, len(tools))
	for _, t := range tools {
		if t != nil && t.Name != "" {
			schema, _ := t.InputSchema.(map[string]any)
			out = append(out, ToolDescriptor{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: schema,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
