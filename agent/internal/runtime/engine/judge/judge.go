package judge

import (
	"github.com/OctoSucker/agent/internal/runtime/store/capability"
	"github.com/OctoSucker/agent/internal/runtime/store/nodefailure"
	procedure "github.com/OctoSucker/agent/internal/runtime/store/procedure"
	"github.com/OctoSucker/agent/internal/runtime/store/recall"
	routinggraph "github.com/OctoSucker/agent/internal/runtime/store/routing_graph"
	"github.com/OctoSucker/agent/internal/runtime/store/task"
	"github.com/OctoSucker/agent/pkg/llmclient"
)

const (
	maxFailsPerTool = 2
	maxFailsPerCap  = 2
)

// Judge owns post-execution evaluation and finalization handlers.
type Judge struct {
	StepCritic       *StepCritic
	TrajectoryCritic *TrajectoryCritic
	Learner          *Learner
	RecallArchiver   *RecallArchiver
}

func NewJudge(
	tasks *task.TaskStore,
	routeGraph *routinggraph.RoutingGraph,
	procedures *procedure.ProcedureRegistry,
	capReg *capability.CapabilityRegistry,
	trajectoryLLM *llmclient.OpenAI,
	recallCorpus *recall.RecallCorpus,
	nodeFailures *nodefailure.NodeFailureStats,
) *Judge {
	learner := &Learner{
		Tasks:      tasks,
		Procedures: procedures,
		RouteGraph: routeGraph,
	}
	archiver := &RecallArchiver{
		Tasks:  tasks,
		Recall: recallCorpus,
	}
	return &Judge{
		StepCritic:       NewStepCritic(tasks, routeGraph, capReg, nodeFailures, maxFailsPerTool, maxFailsPerCap),
		TrajectoryCritic: NewTrajectoryCritic(tasks, trajectoryLLM),
		Learner:          learner,
		RecallArchiver:   archiver,
	}
}
