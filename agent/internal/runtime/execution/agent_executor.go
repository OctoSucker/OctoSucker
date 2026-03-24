package execution

import (
	"github.com/OctoSucker/agent/internal/runtime/store/capability"
	routinggraph "github.com/OctoSucker/agent/internal/runtime/store/routing_graph"
	"github.com/OctoSucker/agent/internal/runtime/store/session"
	"github.com/OctoSucker/agent/pkg/mcpclient"
)

// AgentExecutor bundles tool and plan execution handlers for the dispatcher.
type AgentExecutor struct {
	ToolExec *ToolExecutor
	PlanExec *PlanExecutor
}

// NewAgentExecutor builds plan and tool executors sharing session, routing graph, capability registry, and MCP.
func NewAgentExecutor(
	sessions *session.SessionStore,
	routeGraph *routinggraph.RoutingGraph,
	capReg *capability.CapabilityRegistry,
	mcp *mcpclient.MCPRouter,
) *AgentExecutor {
	return &AgentExecutor{
		ToolExec: &ToolExecutor{
			Sessions: sessions,
			Invoker:  mcp,
		},
		PlanExec: &PlanExecutor{
			Sessions:    sessions,
			RouteGraph:  routeGraph,
			CapRegistry: capReg,
			MCPRouter:   mcp,
		},
	}
}
