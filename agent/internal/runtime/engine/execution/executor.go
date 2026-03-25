package execution

import (
	"github.com/OctoSucker/agent/internal/runtime/store/capability"
	routinggraph "github.com/OctoSucker/agent/internal/runtime/store/routing_graph"
	"github.com/OctoSucker/agent/internal/runtime/store/task"
	"github.com/OctoSucker/agent/pkg/mcpclient"
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

// NewAgentExecutor builds plan and tool executors sharing session, routing graph, capability registry, and MCP.
func NewAgentExecutor(
	sessions *task.TaskStore,
	routeGraph *routinggraph.RoutingGraph,
	capReg *capability.CapabilityRegistry,
	mcp *mcpclient.MCPRouter,
) *AgentExecutor {
	return &AgentExecutor{
		ToolExec: &ToolExecutor{
			Tasks:   sessions,
			Invoker: mcp,
		},
		PlanExec: &PlanExecutor{
			Tasks:       sessions,
			RouteGraph:  routeGraph,
			CapRegistry: capReg,
		},
	}
}
