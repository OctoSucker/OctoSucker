package judge

import (
	"github.com/OctoSucker/agent/internal/runtime/store/capability"
	"github.com/OctoSucker/agent/internal/runtime/store/nodefailure"
	routinggraph "github.com/OctoSucker/agent/internal/runtime/store/routing_graph"
	"github.com/OctoSucker/agent/internal/runtime/store/task"
	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
)

type StepCritic struct {
	Tasks                 *task.TaskStore
	RouteGraph            *routinggraph.RoutingGraph
	CapRegistry           *capability.CapabilityRegistry
	NodeFailures          *nodefailure.NodeFailureStats
	MaxFailsPerTool       int
	MaxFailsPerCapability int // per stepID+capability failure count before preferring switch (0 = use default 2)
}

func NewStepCritic(tasks *task.TaskStore, routeGraph *routinggraph.RoutingGraph, capRegistry *capability.CapabilityRegistry, nodeFailures *nodefailure.NodeFailureStats, maxFailsPerTool int, maxFailsPerCapability int) *StepCritic {
	return &StepCritic{Tasks: tasks, RouteGraph: routeGraph, CapRegistry: capRegistry, NodeFailures: nodeFailures, MaxFailsPerTool: maxFailsPerTool, MaxFailsPerCapability: maxFailsPerCapability}
}

func (x *StepCritic) maxStepToolFails() int {
	if x.MaxFailsPerTool <= 0 {
		panic("evaluation.StepCritic: MaxFailsPerTool must be > 0")
	}
	return x.MaxFailsPerTool
}

func (x *StepCritic) maxCapabilityFails() int {
	if x.MaxFailsPerCapability < 1 {
		return 2
	}
	return x.MaxFailsPerCapability
}

func (x *StepCritic) bumpCapabilityFailCount(sess *ports.Task, stepID, capability string) int {
	if sess.CapabilityFailCount == nil {
		sess.CapabilityFailCount = make(map[string]int)
	}
	k := rtutils.CapabilityFailCountKey(stepID, capability)
	sess.CapabilityFailCount[k]++
	return sess.CapabilityFailCount[k]
}
