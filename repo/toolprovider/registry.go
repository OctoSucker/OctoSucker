// Package toolprovider: tool Provider implementations (builtin + MCP), Registry, MCP sessions; Invoke returns types.ToolResult.
package toolprovider

import (
	"context"
	"fmt"
	"sort"

	"github.com/OctoSucker/octosucker/config"
	"github.com/OctoSucker/octosucker/engine/types"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
	catalogbuiltin "github.com/OctoSucker/octosucker/repo/toolprovider/builtin/catalog"
	cronjobbuiltin "github.com/OctoSucker/octosucker/repo/toolprovider/builtin/cronjob"
	execbuiltin "github.com/OctoSucker/octosucker/repo/toolprovider/builtin/exec"
	kggraph "github.com/OctoSucker/octosucker/repo/toolprovider/builtin/kg_graph"
	skillsbuiltin "github.com/OctoSucker/octosucker/repo/toolprovider/builtin/skills"
	thinkerbuiltin "github.com/OctoSucker/octosucker/repo/toolprovider/builtin/thinker"
	telegrambuiltin "github.com/OctoSucker/octosucker/repo/toolprovider/builtin/telegram"
	mcpstore "github.com/OctoSucker/octosucker/repo/toolprovider/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Registry loads Providers, enforces globally unique MCP tool names, and dispatches by flat tool id.
type Registry struct {
	// ProvidersMap:
	//   key:   Provider.Name() id — stable tool-provider name per backend (e.g. "skills", "exec", MCP server name).
	//   value: that backend's Provider implementation.
	ProvidersMap map[string]Provider

	// toolToProvider:
	//   key:   globally unique tool name (flat id passed to Invoke).
	//   value: Provider that owns and handles that tool (populated by reindexTools).
	toolToProvider map[string]Provider

	SkillsProvider *skillsbuiltin.Runner
}

func NewRegistry(
	ctx context.Context,
	mcpEndpoints []string,
	execCfg config.Exec,
	telegramCfg config.Telegram,
	skillsDir string,
	embedLLM *llmclient.OpenAI,
) (*Registry, error) {

	r := &Registry{
		ProvidersMap: map[string]Provider{},
	}

	execB, err := execbuiltin.NewRunner(execCfg)
	if err != nil {
		return nil, err
	}
	execID, _ := execB.Name()
	r.ProvidersMap[execID] = execB

	tg, err := telegrambuiltin.NewRunner(telegramCfg)
	if err != nil {
		return nil, fmt.Errorf("tool registry: telegram builtin: %w", err)
	}
	tgID, _ := tg.Name()
	r.ProvidersMap[tgID] = tg

	skillRunner, err := skillsbuiltin.NewRunner(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("tool registry: skills runner: %w", err)
	}
	skillID, _ := skillRunner.Name()
	r.ProvidersMap[skillID] = skillRunner
	r.SkillsProvider = skillRunner

	catalogRunner, err := catalogbuiltin.NewRunner(embedLLM)
	if err != nil {
		return nil, fmt.Errorf("tool registry: catalog runner: %w", err)
	}
	catID, _ := catalogRunner.Name()
	r.ProvidersMap[catID] = catalogRunner

	thinkerRunner, err := thinkerbuiltin.NewRunner(embedLLM)
	if err != nil {
		return nil, fmt.Errorf("tool registry: thinker builtin: %w", err)
	}
	thinkID, _ := thinkerRunner.Name()
	r.ProvidersMap[thinkID] = thinkerRunner

	if len(execCfg.WorkspaceDirs) == 0 {
		return nil, fmt.Errorf("tool registry: exec.workspace_dirs is required for cronjob builtin")
	}
	cj, err := cronjobbuiltin.NewRunner(execCfg.WorkspaceDirs[0])
	if err != nil {
		return nil, fmt.Errorf("tool registry: cronjob builtin: %w", err)
	}
	cjID, _ := cj.Name()
	r.ProvidersMap[cjID] = cj

	kg, err := kggraph.NewRunner(execCfg.WorkspaceDirs[0], embedLLM)
	if err != nil {
		return nil, fmt.Errorf("tool registry: kg_graph builtin: %w", err)
	}
	kgID, _ := kg.Name()
	r.ProvidersMap[kgID] = kg

	for _, ep := range mcpEndpoints {
		sess, err := mcpstore.NewRemoteSession(ctx, ep)
		if err != nil {
			return nil, fmt.Errorf("tool registry: connect mcp endpoint %q: %w", ep, err)
		}
		sid, _ := sess.Name()
		r.ProvidersMap[sid] = sess
	}

	intro := newIntrospectionBackend(r)
	introID, _ := intro.Name()
	r.ProvidersMap[introID] = intro

	if err := r.reindexTools(ctx); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Registry) reindexTools(ctx context.Context) error {
	r.toolToProvider = make(map[string]Provider)
	ids := make([]string, 0, len(r.ProvidersMap))
	for k := range r.ProvidersMap {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	for _, pid := range ids {
		p := r.ProvidersMap[pid]
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
	return p.Invoke(ctx, tool, arguments)
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
