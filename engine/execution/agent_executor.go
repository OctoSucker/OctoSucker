package execution

import (
	"github.com/OctoSucker/octosucker/repo/toolprovider"
	"github.com/OctoSucker/octosucker/repo/taskstore"
)

type PlanExecutor struct {
	Tasks *taskstore.TaskStore
}

// AgentExecutor bundles tool and plan execution handlers for the dispatcher.
type AgentExecutor struct {
	ToolExec *ToolExecutor
	PlanExec *PlanExecutor
}

// NewAgentExecutor builds plan and tool executors sharing task store and tool registry (invoke).
func NewAgentExecutor(
	tasks *taskstore.TaskStore,
	toolRegistry *toolprovider.Registry,
) *AgentExecutor {
	return &AgentExecutor{
		ToolExec: &ToolExecutor{
			Tasks:        tasks,
			ToolRegistry: toolRegistry,
		},
		PlanExec: &PlanExecutor{Tasks: tasks},
	}
}
