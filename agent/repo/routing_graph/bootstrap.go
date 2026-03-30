package routinggraph

import (
	"context"
	"errors"
	"fmt"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/model"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/OctoSucker/agent/repo/capability"
	catalogbuiltin "github.com/OctoSucker/agent/repo/capability/builtin/catalog"
	skillsbuiltin "github.com/OctoSucker/agent/repo/capability/builtin/skills"
	"github.com/OctoSucker/agent/repo/graph"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// New loads capabilities from runners, scans on-disk skills (*.md), resyncs registry tool lists, and builds static topology + SQLite-backed edge state.
func New(
	ctx context.Context,
	mcpEndpoints []string,
	execCfg config.Exec,
	telegramCfg config.Telegram,
	skillsDir string,
	plannerLLM *llmclient.OpenAI,
	store *model.AgentDB,
) (*RoutingGraph, error) {
	skillRunner, err := skillsbuiltin.NewRunner(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("routinggraph: skills runner: %w", err)
	}
	catalogRunner, err := catalogbuiltin.NewRunner(plannerLLM)
	if err != nil {
		return nil, fmt.Errorf("routinggraph: catalog runner: %w", err)
	}
	reg, err := capability.NewCapabilityRegistry(ctx, mcpEndpoints, execCfg, telegramCfg, skillRunner, catalogRunner)
	if err != nil {
		return nil, fmt.Errorf("routinggraph: capability registry: %w", err)
	}
	if err := reg.ResyncToolsFromRunners(ctx); err != nil {
		return nil, fmt.Errorf("routinggraph: resync tools: %w", err)
	}
	static := staticAdjacencyFromCapabilities(reg.AllCapabilities())
	g, err := graph.New(static, store)
	if err != nil {
		return nil, err
	}
	if err := g.LoadFromDB(); err != nil {
		return nil, err
	}
	rg := &RoutingGraph{reg: reg, g: g}
	return rg, nil
}

func (s *RoutingGraph) requireRegistry() error {
	if s == nil || s.reg == nil {
		return errors.New("routinggraph: capability registry is not set")
	}
	return nil
}

// Invoke runs a tool through the embedded capability registry.
func (s *RoutingGraph) Invoke(ctx context.Context, inv ports.CapabilityInvocation) (ports.ToolResult, error) {
	if err := s.requireRegistry(); err != nil {
		return ports.ToolResult{}, err
	}
	return s.reg.Invoke(ctx, inv)
}

// AllCapabilities returns the current capability → tool name snapshot from the registry.
func (s *RoutingGraph) AllCapabilities() (map[string]ports.Capability, error) {
	if err := s.requireRegistry(); err != nil {
		return nil, err
	}
	return s.reg.AllCapabilities(), nil
}

// PlannerToolAppendix builds the planner-facing tool listing (names + JSON Schema snippets).
func (s *RoutingGraph) PlannerToolAppendix() (string, error) {
	if err := s.requireRegistry(); err != nil {
		return "", err
	}
	return s.reg.PlannerToolAppendix(), nil
}

func (s *RoutingGraph) PlannerSkills() (skillsbuiltin.PromptBundle, error) {
	if err := s.requireRegistry(); err != nil {
		return skillsbuiltin.PromptBundle{}, err
	}
	return s.reg.PlannerSkills(), nil
}

// Tool resolves MCP tool metadata for a capability/tool pair.
func (s *RoutingGraph) Tool(capID, tool string) (*mcp.Tool, error) {
	if err := s.requireRegistry(); err != nil {
		return nil, err
	}
	return s.reg.Tool(capID, tool)
}
