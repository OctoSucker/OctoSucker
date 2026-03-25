package judge

import (
	"database/sql"

	"github.com/OctoSucker/agent/internal/runtime/store/capability"
	"github.com/OctoSucker/agent/internal/runtime/store/nodefailure"
	"github.com/OctoSucker/agent/internal/runtime/store/recall"
	routinggraph "github.com/OctoSucker/agent/internal/runtime/store/routing_graph"
	skill "github.com/OctoSucker/agent/internal/runtime/store/skill"
	"github.com/OctoSucker/agent/internal/runtime/store/task"
	"github.com/OctoSucker/agent/pkg/llmclient"
)

const (
	maxFailsPerTool       = 2
	maxFailsPerCap        = 2
	skillRouteThreshold   = 0.9
	extractScoreThreshold = 0.8
)

// Judge owns post-execution evaluation and finalization handlers.
type Judge struct {
	StepCritic       *StepCritic
	TrajectoryCritic *TrajectoryCritic
	Learner          *Learner
	RecallArchiver   *RecallArchiver
}

func New(
	tasks *task.TaskStore,
	routeGraph *routinggraph.RoutingGraph,
	skills *skill.SkillRegistry,
	capReg *capability.CapabilityRegistry,
	trajectoryLLM *llmclient.OpenAI,
	recallCorpus *recall.RecallCorpus,
	nodeFailures *nodefailure.NodeFailureStats,
	sqlDB *sql.DB,
	skillLearnMinPlanSteps int,
	skillLearnMinSuccessCount int,
) *Judge {
	learner := &Learner{
		Tasks:                          tasks,
		Skills:                         skills,
		RouteGraph:                     routeGraph,
		SkillRouteThreshold:            skillRouteThreshold,
		ExtractScoreThreshold:          extractScoreThreshold,
		SQLDB:                          sqlDB,
		MinPlanStepsForSkillExtract:    skillLearnMinPlanSteps,
		MinQualifyingSuccessesForSkill: skillLearnMinSuccessCount,
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
