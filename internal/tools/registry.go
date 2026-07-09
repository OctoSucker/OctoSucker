// Package toolprovider: tool Provider implementations (builtin + MCP), Registry, MCP sessions; Invoke returns types.ToolResult.
package tools

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/OctoSucker/octosucker/config"
	types "github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/storage"
	skillsbuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/skills"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Registry loads Providers, enforces globally unique MCP tool names, and dispatches by flat tool id.
type Registry struct {
	// providersByName:
	//   key:   Provider.Name() id — stable tool-provider name per backend (e.g. "skills", "exec", MCP server name).
	//   value: that backend's Provider implementation.
	providersByName map[string]Provider

	// toolToProvider:
	//   key:   globally unique tool name (flat id passed to Invoke).
	//   value: Provider that owns and handles that tool (populated by reindexTools).
	toolToProvider map[string]Provider

	skillsProvider *skillsbuiltin.Runner
}

type RegistryDeps struct {
	WorkspaceRoot string
	MCPEndpoints  []string
	OpenAI        config.OpenAI
	Exec          config.Exec
	Telegram      config.Telegram
	SkillsDir     string
	EmbedLLM      *llmclient.OpenAI
	Data          *storage.DB
}

func NewRegistry(ctx context.Context, deps RegistryDeps) (*Registry, error) {

	r := &Registry{
		providersByName: map[string]Provider{},
	}

	if err := r.registerBuiltins(deps.WorkspaceRoot, deps.OpenAI, deps.Data, deps.Exec, deps.Telegram, deps.SkillsDir, deps.EmbedLLM); err != nil {
		return nil, err
	}
	if err := r.registerMCPProviders(ctx, deps.MCPEndpoints); err != nil {
		return nil, err
	}

	if err := r.reindexTools(ctx); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Registry) reindexTools(ctx context.Context) error {
	r.toolToProvider = make(map[string]Provider)
	ids := r.providerIDs()
	for _, pid := range ids {
		p := r.providersByName[pid]
		tools, err := p.ToolList(ctx)
		if err != nil {
			return fmt.Errorf("tool registry: list tools for provider %q: %w", pid, err)
		}
		for _, t := range tools {
			if t == nil || t.Name == "" {
				continue
			}
			if prev, ok := r.toolToProvider[t.Name]; ok {
				prevID, _ := prev.Name()
				return fmt.Errorf("tool registry: duplicate tool name %q (providers %q and %q)", t.Name, prevID, pid)
			}
			r.toolToProvider[t.Name] = p
		}
	}
	return nil
}

func (r *Registry) Invoke(ctx context.Context, tool string, arguments map[string]any) (types.ToolResult, error) {
	p, ok := r.toolToProvider[tool]
	if !ok {
		return types.ToolResult{Err: fmt.Errorf("tool registry: unknown tool %q", tool)}, fmt.Errorf("tool registry: unknown tool %q", tool)
	}
	if err := preflightTool(tool, arguments); err != nil {
		return types.ToolResult{Err: err}, err
	}
	return p.Invoke(ctx, tool, arguments)
}

func (r *Registry) Assess(tool string, arguments map[string]any) types.ToolPolicy {
	a := AssessToolCall(tool, arguments)
	return types.ToolPolicy{
		Capabilities: a.Capabilities,
		Risk:         a.Risk,
		Summary:      a.Summary,
	}
}

// AllToolIDs returns sorted flat tool names for routing topology.
func (r *Registry) AllToolIDs() []string {
	ids := make([]string, 0, len(r.toolToProvider))
	for name := range r.toolToProvider {
		ids = append(ids, name)
	}
	sort.Strings(ids)
	return ids
}

func (r *Registry) Tool(name string) (*mcp.Tool, error) {
	p, ok := r.toolToProvider[name]
	if !ok {
		return nil, fmt.Errorf("tool registry: unknown tool %q", name)
	}
	t, err := p.Tool(name)
	if err != nil {
		return nil, fmt.Errorf("tool registry: get tool %q: %w", name, err)
	}
	return t, nil
}

func (r *Registry) SkillDescriptors() []map[string]any {
	if r == nil || r.skillsProvider == nil {
		return nil
	}
	all := r.skillsProvider.AllSkills()
	out := make([]map[string]any, 0, len(all))
	for _, sk := range all {
		item := map[string]any{
			"name":        sk.Name,
			"description": sk.Description,
			"summary":     sk.Summary,
			"source_file": sk.SourceFile,
			"byte_size":   sk.ByteSize,
		}
		if sk.CLIPlugin != nil {
			toolNames := make([]string, 0, len(sk.CLIPlugin.Tools))
			for _, tool := range sk.CLIPlugin.Tools {
				toolNames = append(toolNames, tool.Name)
			}
			item["plugin_provider"] = sk.CLIPlugin.Provider
			item["plugin_tools"] = toolNames
		}
		out = append(out, item)
	}
	return out
}

func (r *Registry) Close() error {
	if r == nil {
		return nil
	}
	var err error
	for _, id := range r.providerIDs() {
		p, ok := r.providerByName(id)
		if !ok {
			continue
		}
		c, ok := p.(ClosableProvider)
		if !ok {
			continue
		}
		if cerr := c.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("tool registry: close provider %q: %w", id, cerr))
		}
	}
	return err
}

func (r *Registry) providerIDs() []string {
	ids := make([]string, 0, len(r.providersByName))
	for id := range r.providersByName {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (r *Registry) providerByName(id string) (Provider, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.providersByName[id]
	return p, ok
}
