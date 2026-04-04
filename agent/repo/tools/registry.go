package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
	catalogbuiltin "github.com/OctoSucker/agent/repo/tools/builtin/catalog"
	cronjobbuiltin "github.com/OctoSucker/agent/repo/tools/builtin/cronjob"
	execbuiltin "github.com/OctoSucker/agent/repo/tools/builtin/exec"
	kggraph "github.com/OctoSucker/agent/repo/tools/builtin/kg_graph"
	skillsbuiltin "github.com/OctoSucker/agent/repo/tools/builtin/skills"
	telegrambuiltin "github.com/OctoSucker/agent/repo/tools/builtin/telegram"
	mcpstore "github.com/OctoSucker/agent/repo/tools/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolRegistry loads ToolProviders, enforces globally unique MCP tool names, and dispatches by flat tool id.
type ToolRegistry struct {
	Backends      map[string]ToolProvider
	toolOwner     map[string]ToolProvider // tool name -> owning provider
	skillsBackend *skillsbuiltin.Runner
}

func NewToolRegistry(
	ctx context.Context,
	mcpEndpoints []string,
	execCfg config.Exec,
	telegramCfg config.Telegram,
	skillsBackend *skillsbuiltin.Runner,
	catalogBackend *catalogbuiltin.Runner,
	embedLLM *llmclient.OpenAI,
) (*ToolRegistry, error) {

	r := &ToolRegistry{
		Backends:      map[string]ToolProvider{},
		skillsBackend: skillsBackend,
	}

	execB, err := execbuiltin.NewRunner(execCfg)
	if err != nil {
		return nil, err
	}
	r.Backends[execB.Name()] = execB

	if strings.TrimSpace(telegramCfg.BotToken) != "" {
		tg, err := telegrambuiltin.NewRunner(telegramCfg)
		if err != nil {
			return nil, fmt.Errorf("tool registry: telegram builtin: %w", err)
		}
		r.Backends[tg.Name()] = tg
	}

	if skillsBackend == nil {
		return nil, fmt.Errorf("tool registry: skills backend is required")
	}
	r.Backends[skillsBackend.Name()] = skillsBackend

	if catalogBackend == nil {
		return nil, fmt.Errorf("tool registry: catalog backend is required")
	}
	r.Backends[catalogBackend.Name()] = catalogBackend

	if len(execCfg.WorkspaceDirs) == 0 {
		return nil, fmt.Errorf("tool registry: exec.workspace_dirs is required for cronjob builtin")
	}
	cj, err := cronjobbuiltin.NewRunner(execCfg.WorkspaceDirs[0])
	if err != nil {
		return nil, fmt.Errorf("tool registry: cronjob builtin: %w", err)
	}
	r.Backends[cj.Name()] = cj

	if embedLLM == nil {
		return nil, fmt.Errorf("tool registry: embed llm is required for kg_graph")
	}
	kg, err := kggraph.NewRunner(execCfg.WorkspaceDirs[0], embedLLM)
	if err != nil {
		return nil, fmt.Errorf("tool registry: kg_graph builtin: %w", err)
	}
	r.Backends[kg.Name()] = kg

	for _, ep := range mcpEndpoints {
		sess, err := mcpstore.NewRemoteSession(ctx, ep)
		if err != nil {
			return nil, fmt.Errorf("tool registry: connect mcp endpoint %q: %w", ep, err)
		}
		r.Backends[sess.Name()] = sess
	}

	intro := newIntrospectionBackend(r)
	r.Backends[intro.Name()] = intro

	if err := r.reindexTools(ctx); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *ToolRegistry) reindexTools(ctx context.Context) error {
	r.toolOwner = make(map[string]ToolProvider)
	keys := make([]string, 0, len(r.Backends))
	for k := range r.Backends {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, bid := range keys {
		p := r.Backends[bid]
		tools, err := p.ToolList(ctx)
		if err != nil {
			return fmt.Errorf("tool registry: list tools for backend %q: %w", bid, err)
		}
		for _, t := range tools {
			if t == nil || t.Name == "" {
				continue
			}
			if prev, ok := r.toolOwner[t.Name]; ok {
				return fmt.Errorf("tool registry: duplicate tool name %q (backends %q and %q)", t.Name, prev.Name(), bid)
			}
			r.toolOwner[t.Name] = p
		}
	}
	return nil
}

func (r *ToolRegistry) PlannerSkills() skillsbuiltin.PromptBundle {
	if r == nil || r.skillsBackend == nil {
		return skillsbuiltin.PromptBundle{}
	}
	return r.skillsBackend.PlannerBundle()
}

func (r *ToolRegistry) ResyncToolsFromBackends(ctx context.Context) error {
	return r.reindexTools(ctx)
}

func (r *ToolRegistry) Invoke(ctx context.Context, inv ports.ToolInvocation) (ports.ToolResult, error) {
	p, ok := r.toolOwner[inv.Tool]
	if !ok {
		return ports.ToolResult{Err: fmt.Errorf("tool registry: unknown tool %q", inv.Tool)}, fmt.Errorf("tool registry: unknown tool %q", inv.Tool)
	}
	return p.Invoke(ctx, inv.Tool, inv.Arguments)
}

// AllToolIDs returns sorted flat tool names for routing topology.
func (r *ToolRegistry) AllToolIDs() []string {
	ids := make([]string, 0, len(r.toolOwner))
	for name := range r.toolOwner {
		ids = append(ids, name)
	}
	sort.Strings(ids)
	return ids
}

func (r *ToolRegistry) Tool(name string) (*mcp.Tool, error) {
	p, ok := r.toolOwner[name]
	if !ok {
		return nil, fmt.Errorf("tool registry: unknown tool %q", name)
	}
	t, err := p.Tool(name)
	if err != nil {
		return nil, fmt.Errorf("tool registry: get tool %q: %w", name, err)
	}
	return t, nil
}

func (r *ToolRegistry) PlannerToolAppendix() string {
	var b strings.Builder
	b.WriteString("Each line is one tool name (use this exact string in plan steps).\n")
	for _, name := range r.AllToolIDs() {
		b.WriteString("- ")
		b.WriteString(name)
		p := r.toolOwner[name]
		if p == nil {
			b.WriteByte('\n')
			continue
		}
		if t, err := p.Tool(name); err == nil {
			raw, err := json.Marshal(t.InputSchema)
			if err != nil {
				return ""
			}
			b.WriteString(" params JSON Schema: ")
			b.Write(raw)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
