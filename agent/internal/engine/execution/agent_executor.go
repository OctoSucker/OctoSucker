package execution

import (
	routinggraph "github.com/OctoSucker/agent/repo/routing_graph"
	"github.com/OctoSucker/agent/repo/task"
)

type PlanExecutor struct {
	Tasks      *task.TaskStore
	RouteGraph *routinggraph.RoutingGraph
}

// AgentExecutor bundles tool and plan execution handlers for the dispatcher.
type AgentExecutor struct {
	ToolExec *ToolExecutor
	PlanExec *PlanExecutor
}

// NewAgentExecutor builds plan and tool executors sharing task store and routing graph (tool calls go through the graph).
func NewAgentExecutor(
	tasks *task.TaskStore,
	routeGraph *routinggraph.RoutingGraph,
) *AgentExecutor {
	return &AgentExecutor{
		ToolExec: &ToolExecutor{
			Tasks:      tasks,
			RouteGraph: routeGraph,
		},
		PlanExec: &PlanExecutor{
			Tasks:      tasks,
			RouteGraph: routeGraph,
		},
	}
}
