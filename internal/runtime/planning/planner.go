package planning

import (
	"github.com/OctoSucker/octosucker/internal/runtime/taskstore"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
)

type Planner struct {
	Tasks        *taskstore.TaskStore
	ToolRegistry ToolCatalog
	RouteGraph   RouteGraph
	PlannerLLM   *llmclient.OpenAI
	ProjectCtx   string
}

// NewPlanner centralizes planner initialization, including system prompt generation.
func NewPlanner(
	tasks *taskstore.TaskStore,
	toolRegistry ToolCatalog,
	routeGraph RouteGraph,
	plannerLLM *llmclient.OpenAI,
	projectCtx string,
) (*Planner, error) {
	return &Planner{
		Tasks:        tasks,
		ToolRegistry: toolRegistry,
		RouteGraph:   routeGraph,
		PlannerLLM:   plannerLLM,
		ProjectCtx:   projectCtx,
	}, nil
}
