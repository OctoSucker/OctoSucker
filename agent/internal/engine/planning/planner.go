package planning

import (
	"github.com/OctoSucker/agent/pkg/llmclient"
	routinggraph "github.com/OctoSucker/agent/repo/routing_graph"
	"github.com/OctoSucker/agent/repo/task"
)

type Planner struct {
	Tasks      *task.TaskStore
	RouteGraph *routinggraph.RoutingGraph
	PlannerLLM *llmclient.OpenAI
}

// NewPlanner centralizes planner initialization, including system prompt generation.
func NewPlanner(
	tasks *task.TaskStore,
	routeGraph *routinggraph.RoutingGraph,
	plannerLLM *llmclient.OpenAI,
) (*Planner, error) {
	return &Planner{
		Tasks:      tasks,
		RouteGraph: routeGraph,
		PlannerLLM: plannerLLM,
	}, nil
}
