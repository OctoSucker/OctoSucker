package engine

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/internal/runtime/cognition"
	"github.com/OctoSucker/agent/internal/runtime/execution"
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

type Dispatcher struct {
	Brain    *cognition.AgentBrain
	Executor *execution.AgentExecutor
	handlers map[string]cognition.EventHandler
	MaxSteps int
}

func NewDispatcher(
	ctx context.Context,
	mcpRouter *mcpclient.MCPRouter,
	openai config.OpenAI,
	sqlDB *sql.DB,
	graphPathMode string,
	skillLearnMinPlanSteps int,
	skillLearnMinSuccessCount int,
) (*Dispatcher, error) {

	capReg, err := capability.NewCapabilityRegistry(ctx, mcpRouter)
	if err != nil {
		return nil, err
	}
	gpm := ports.ParseGraphPathMode(graphPathMode)
	sessions := session.NewSessionStore(sqlDB)
	routeGraph := routinggraph.NewRoutingGraphFromCapabilityRegistry(capReg, sqlDB)
	skills := skill.NewSkillRegistry(sqlDB)
	nodeFailures := nodefailure.NewNodeFailureStats(sqlDB)

	plannerLLM := llmclient.NewOpenAI(openai.BaseURL, openai.APIKey, openai.Model, openai.EmbeddingModel)
	embedder := llmclient.NewOpenAI(openai.BaseURL, openai.APIKey, openai.Model, openai.EmbeddingModel)
	trajectoryLLM := llmclient.NewOpenAI(openai.BaseURL, openai.APIKey, openai.Model, openai.EmbeddingModel)
	recallCorpus := recall.NewRecallCorpus(embedder, sqlDB)

	d := &Dispatcher{
		handlers: map[string]cognition.EventHandler{},
		MaxSteps: 200,
		Brain: cognition.NewAgentBrain(
			sessions,
			routeGraph,
			skills,
			capReg,
			mcpRouter,
			plannerLLM,
			embedder,
			trajectoryLLM,
			recallCorpus,
			nodeFailures,
			sqlDB,
			gpm,
			skillLearnMinPlanSteps,
			skillLearnMinSuccessCount,
		),
		Executor: execution.NewAgentExecutor(
			sessions,
			routeGraph,
			capReg,
			mcpRouter,
		),
	}

	d.registerDefaultHandlers()
	return d, nil
}

func (d *Dispatcher) registerDefaultHandlers() {
	if d == nil {
		return
	}
	d.handlers = map[string]cognition.EventHandler{
		ports.EvUserInput:           d.Brain.Planner.HandleUserInput,
		ports.EvPlanCreated:         d.Executor.PlanExec.HandlePlanCreated,
		ports.EvStepCompleted:       d.Executor.PlanExec.HandleStepCompleted,
		ports.EvStepCapabilityRetry: d.Executor.PlanExec.HandleStepCapabilityRetry,
		ports.EvToolCall:            d.Executor.ToolExec.HandleToolCall,
		ports.EvObservationReady:    d.Brain.StepCritic.HandleObservationReady,
		ports.EvTrajectoryCheck:     d.Brain.TrajectoryCritic.HandleTrajectoryCheck,
		ports.EvTurnFinalized:       d.Brain.TurnFinalized.HandleTurnFinalized,
	}
}

func (d *Dispatcher) Run(ctx context.Context, queue []ports.Event) error {
	if d == nil {
		return fmt.Errorf("engine: nil dispatcher")
	}
	n := 0
	for len(queue) > 0 && n < d.MaxSteps {
		evt := queue[0]
		queue = queue[1:]
		n++
		h, ok := d.handlers[evt.Type]
		if !ok {
			continue
		}
		out, err := h(ctx, evt)
		if err != nil {
			log.Printf("engine.Dispatcher.Run: abort event=%s iter=%d err=%v", evt.Type, n, err)
			return err
		}
		queue = append(queue, out...)
	}
	return nil
}
