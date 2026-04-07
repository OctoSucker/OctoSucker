package planning

import (
	"github.com/OctoSucker/octosucker/repo/routegraph"
	"github.com/OctoSucker/octosucker/repo/toolprovider"
	"github.com/OctoSucker/octosucker/repo/taskstore"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
)

type Planner struct {
	Tasks        *taskstore.TaskStore
	ToolRegistry *toolprovider.Registry
	RouteGraph   *routegraph.Graph
	PlannerLLM   *llmclient.OpenAI
}

// NewPlanner centralizes planner initialization, including system prompt generation.
func NewPlanner(
	tasks *taskstore.TaskStore,
	toolRegistry *toolprovider.Registry,
	routeGraph *routegraph.Graph,
	plannerLLM *llmclient.OpenAI,
) (*Planner, error) {
	return &Planner{
		Tasks:        tasks,
		ToolRegistry: toolRegistry,
		RouteGraph:   routeGraph,
		PlannerLLM:   plannerLLM,
	}, nil
}
