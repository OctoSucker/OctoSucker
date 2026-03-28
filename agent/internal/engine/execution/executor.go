package execution

import (
	"github.com/OctoSucker/agent/internal/store/capability"
	routinggraph "github.com/OctoSucker/agent/internal/store/routing_graph"
	"github.com/OctoSucker/agent/internal/store/task"
)

type PlanExecutor struct {
	Tasks       *task.TaskStore
	RouteGraph  *routinggraph.RoutingGraph
	CapRegistry *capability.CapabilityRegistry
}

// AgentExecutor bundles tool and plan execution handlers for the dispatcher.
type AgentExecutor struct {
	ToolExec *ToolExecutor
	PlanExec *PlanExecutor
}

// NewAgentExecutor builds plan and tool executors sharing task store, routing graph, capability registry, and MCP router.
func NewAgentExecutor(
	tasks *task.TaskStore,
	routeGraph *routinggraph.RoutingGraph,
	capReg *capability.CapabilityRegistry,
) *AgentExecutor {
	return &AgentExecutor{
		ToolExec: &ToolExecutor{
			Tasks:   tasks,
			Invoker: capReg,
		},
		PlanExec: &PlanExecutor{
			Tasks:       tasks,
			RouteGraph:  routeGraph,
			CapRegistry: capReg,
		},
	}
}
