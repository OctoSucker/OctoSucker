package routinggraph

import (
	"context"
	"errors"
	"fmt"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/model"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/OctoSucker/agent/repo/tools"
	catalogbuiltin "github.com/OctoSucker/agent/repo/tools/builtin/catalog"
	skillsbuiltin "github.com/OctoSucker/agent/repo/tools/builtin/skills"
	"github.com/OctoSucker/agent/repo/graph"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// New loads tool runners, scans on-disk skills (*.md), resyncs the tool registry, and builds static topology + SQLite-backed edge state.
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
	reg, err := tools.NewToolRegistry(ctx, mcpEndpoints, execCfg, telegramCfg, skillRunner, catalogRunner, plannerLLM)
	if err != nil {
		return nil, fmt.Errorf("routinggraph: tool registry: %w", err)
	}
	if err := reg.ResyncToolsFromBackends(ctx); err != nil {
		return nil, fmt.Errorf("routinggraph: resync tools: %w", err)
	}
	static := staticAdjacencyFromToolIDs(reg.AllToolIDs())
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
		return errors.New("routinggraph: tool registry is not set")
	}
	return nil
}

// Invoke runs a tool through the embedded tool registry.
func (s *RoutingGraph) Invoke(ctx context.Context, inv ports.ToolInvocation) (ports.ToolResult, error) {
	if err := s.requireRegistry(); err != nil {
		return ports.ToolResult{Err: err}, err
	}
	return s.reg.Invoke(ctx, inv)
}

// PlannerToolAppendix builds the planner-facing tool listing (names + JSON Schema snippets).
func (s *RoutingGraph) PlannerToolAppendix() (string, error) {
	if err := s.requireRegistry(); err != nil {
		return "", err
	}
	return s.reg.PlannerToolAppendix(), nil
}

func (s *RoutingGraph) PlannerSkills() (string, error) {
	if err := s.requireRegistry(); err != nil {
		return "", err
	}
	bundle := s.reg.PlannerSkills()
	return bundle.FormatPromptAppendix(), nil
}

// Tool resolves MCP tool metadata by flat tool name.
func (s *RoutingGraph) Tool(toolName string) (*mcp.Tool, error) {
	if err := s.requireRegistry(); err != nil {
		return nil, err
	}
	return s.reg.Tool(toolName)
}
