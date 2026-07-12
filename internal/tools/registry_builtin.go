package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	types "github.com/OctoSucker/octosucker/internal/toolcontract"
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

// Name is the stable tool-provider name (Registry provider id); not an MCP tool name.
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
			"tool": map[string]any{
				"type":        "string",
				"description": "Optional exact tool name. Provide it when one schema is needed to avoid loading the provider's entire catalog.",
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
			Description: "List tool descriptors exposed by one provider, optionally filtered to one exact tool name.",
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
		requestedTool, err := optionalStringArgument(arguments, "tool")
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		if requestedTool != "" {
			filtered := tools[:0]
			for _, descriptor := range tools {
				if descriptor.Name == requestedTool {
					filtered = append(filtered, descriptor)
				}
			}
			tools = filtered
			if len(tools) == 0 {
				err := fmt.Errorf("tool registry: provider %q has no tool named %q", prov, requestedTool)
				return types.ToolResult{Err: err}, err
			}
		}
		return types.ToolResult{
			Output: map[string]any{
				"provider": prov,
				"tool":     requestedTool,
				"tools":    tools,
			},
		}, nil
	default:
		return types.ToolResult{Err: fmt.Errorf("registry: unknown tool %q", localTool)}, fmt.Errorf("registry: unknown tool %q", localTool)
	}
}

func optionalStringArgument(arguments map[string]any, key string) (string, error) {
	if arguments == nil {
		return "", nil
	}
	value, ok := arguments[key]
	if !ok || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("tool_registry: argument %q must be a string", key)
	}
	return strings.TrimSpace(text), nil
}

// ProviderDescriptors returns every registered provider’s id and description, sorted by id.
func (r *Registry) ProviderDescriptors() []ProviderDescriptor {
	ids := r.providerIDs()
	out := make([]ProviderDescriptor, 0, len(ids))
	for _, id := range ids {
		p, _ := r.providerByName(id)
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

type ToolDescriptor = types.ToolDescriptor

// ToolDescriptorsForProvider returns sorted tool descriptors exposed by the named provider id.
func (r *Registry) ToolDescriptorsForProvider(ctx context.Context, providerName string) ([]ToolDescriptor, error) {
	_ = ctx
	pname := strings.TrimSpace(providerName)
	if pname == "" {
		return nil, fmt.Errorf("tool registry: provider name is required")
	}
	_, ok := r.providerByName(pname)
	if !ok {
		return nil, fmt.Errorf("tool registry: unknown tool provider %q", pname)
	}
	out := make([]ToolDescriptor, 0)
	for name, owner := range r.toolToProvider {
		ownerID, _ := owner.Name()
		if ownerID != pname {
			continue
		}
		t := r.toolSpecs[name]
		if t != nil && t.Name != "" {
			schema, err := normalizeInputSchema(t.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("tool registry: tool %q input schema: %w", t.Name, err)
			}
			policy := r.Assess(t.Name, nil)
			out = append(out, ToolDescriptor{
				Provider:     pname,
				Name:         t.Name,
				Description:  t.Description,
				Capabilities: policy.Capabilities,
				Risk:         policy.Risk,
				OutputTrust:  policy.OutputTrust,
				InputSchema:  schema,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func normalizeInputSchema(schema any) (map[string]any, error) {
	if schema == nil {
		return nil, fmt.Errorf("missing schema")
	}
	if value, ok := schema.(map[string]any); ok {
		return value, nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value, nil
}

// ToolDescriptors returns the complete planner-facing catalog. Provider
// discovery remains available as a user tool, but the planner never needs to
// execute discovery merely to learn its own runtime capabilities.
func (r *Registry) ToolDescriptors(ctx context.Context) ([]ToolDescriptor, error) {
	_ = ctx
	var out []ToolDescriptor
	for _, provider := range r.providerIDs() {
		descriptors, err := r.ToolDescriptorsForProvider(ctx, provider)
		if err != nil {
			return nil, err
		}
		out = append(out, descriptors...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider == out[j].Provider {
			return out[i].Name < out[j].Name
		}
		return out[i].Provider < out[j].Provider
	})
	return out, nil
}
