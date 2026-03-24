package cognition

import (
	"database/sql"

	"github.com/OctoSucker/agent/internal/runtime/cognition/evaluation"
	"github.com/OctoSucker/agent/internal/runtime/cognition/planning"
	"github.com/OctoSucker/agent/internal/runtime/cognition/turnfinalized"
	"github.com/OctoSucker/agent/internal/runtime/store/capability"
	"github.com/OctoSucker/agent/internal/runtime/store/nodefailure"
	"github.com/OctoSucker/agent/internal/runtime/store/recall"
	routinggraph "github.com/OctoSucker/agent/internal/runtime/store/routing_graph"
	"github.com/OctoSucker/agent/internal/runtime/store/session"
	skill "github.com/OctoSucker/agent/internal/runtime/store/skill"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/mcpclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

const (
	skillRouteThreshold   = 0.9
	graphRouteThreshold   = 0.7
	keywordConfidence     = 0.92
	maxFailsPerTool       = 2
	maxFailsPerCap        = 2
	extractScoreThreshold = 0.8
)

// AgentBrain collects cognitive dependencies (planning, critics, learning, recall).
type AgentBrain struct {
	Planner          *planning.Planner
	StepCritic       *evaluation.StepCritic
	TrajectoryCritic *evaluation.TrajectoryCritic
	TurnFinalized    *turnfinalized.Handler
}

// NewAgentBrain wires planner, critics, learner, and recall archiver from shared stores and LLM clients.
func NewAgentBrain(
	sessions *session.SessionStore,
	routeGraph *routinggraph.RoutingGraph,
	skills *skill.SkillRegistry,
	capReg *capability.CapabilityRegistry,
	mcpRouter *mcpclient.MCPRouter,
	plannerLLM *llmclient.OpenAI,
	embedder *llmclient.OpenAI,
	trajectoryLLM *llmclient.OpenAI,
	recallCorpus *recall.RecallCorpus,
	nodeFailures *nodefailure.NodeFailureStats,
	sqlDB *sql.DB,
	gpm ports.GraphPathMode,
	skillLearnMinPlanSteps int,
	skillLearnMinSuccessCount int,
) *AgentBrain {

	return &AgentBrain{
		Planner:          planning.NewPlanner(skillRouteThreshold, graphRouteThreshold, keywordConfidence, sessions, routeGraph, skills, nodeFailures, embedder, recallCorpus, plannerLLM, capReg.AllCapabilities(), mcpclient.PlannerToolAppendix(mcpRouter.CachedToolSpecs()), capReg.ToolInputSchemasByName(), gpm),
		StepCritic:       evaluation.NewStepCritic(sessions, routeGraph, capReg, nodeFailures, maxFailsPerTool, maxFailsPerCap),
		TrajectoryCritic: evaluation.NewTrajectoryCritic(sessions, trajectoryLLM),
		TurnFinalized:    turnfinalized.NewHandler(sessions, skills, routeGraph, embedder, skillRouteThreshold, extractScoreThreshold, sqlDB, skillLearnMinPlanSteps, skillLearnMinSuccessCount, recallCorpus),
	}
}
