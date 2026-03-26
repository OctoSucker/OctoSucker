package engine

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/internal/runtime/engine/execution"
	judgepkg "github.com/OctoSucker/agent/internal/runtime/engine/judge"
	"github.com/OctoSucker/agent/internal/runtime/engine/planning"
	"github.com/OctoSucker/agent/internal/runtime/store/capability"
	"github.com/OctoSucker/agent/internal/runtime/store/nodefailure"
	procedure "github.com/OctoSucker/agent/internal/runtime/store/procedure"
	"github.com/OctoSucker/agent/internal/runtime/store/recall"
	routinggraph "github.com/OctoSucker/agent/internal/runtime/store/routing_graph"
	"github.com/OctoSucker/agent/internal/runtime/store/task"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/mcpclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

type Dispatcher struct {
	Planner  *planning.Planner
	Judge    *judgepkg.Judge
	Executor *execution.AgentExecutor
	MaxSteps int
}

func NewDispatcher(
	ctx context.Context,
	mcpRouter *mcpclient.MCPRouter,
	openai config.OpenAI,
	sqlDB *sql.DB,
) (*Dispatcher, error) {

	capReg, err := capability.NewCapabilityRegistry(ctx, mcpRouter)
	if err != nil {
		return nil, err
	}
	taskStore, err := task.NewTaskStore(sqlDB)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: task store: %w", err)
	}
	routeGraph, err := routinggraph.NewRoutingGraphFromCapabilityRegistry(capReg, sqlDB)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: routing graph: %w", err)
	}
	nodeFailures, err := nodefailure.NewNodeFailureStats(sqlDB)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: node failures: %w", err)
	}

	plannerLLM := llmclient.NewOpenAI(openai.BaseURL, openai.APIKey, openai.Model, openai.EmbeddingModel)
	embedder := llmclient.NewOpenAI(openai.BaseURL, openai.APIKey, openai.Model, openai.EmbeddingModel)
	trajectoryLLM := llmclient.NewOpenAI(openai.BaseURL, openai.APIKey, openai.Model, openai.EmbeddingModel)
	procedures, err := procedure.NewProcedureRegistry(sqlDB, embedder)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: procedure registry: %w", err)
	}
	recallCorpus, err := recall.NewRecallCorpus(embedder, sqlDB)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: recall corpus: %w", err)
	}

	d := &Dispatcher{
		MaxSteps: 200,
		Planner: planning.NewPlanner(
			taskStore,
			routeGraph,
			procedures,
			nodeFailures,
			recallCorpus,
			plannerLLM,
			capReg,
			mcpRouter.PlannerToolAppendix(),
		),
		Judge: judgepkg.NewJudge(
			taskStore,
			routeGraph,
			procedures,
			capReg,
			trajectoryLLM,
			recallCorpus,
			nodeFailures,
		),
		Executor: execution.NewAgentExecutor(
			taskStore,
			routeGraph,
			capReg,
			mcpRouter,
		),
	}

	return d, nil
}

func (d *Dispatcher) Run(ctx context.Context, event ports.Event) error {
	evt := event
	for n := 1; n <= d.MaxSteps; n++ {
		var (
			out *ports.Event
			err error
		)
		switch evt.Type {
		case ports.EvUserInput:
			out, err = d.Planner.HandleUserInput(ctx, evt)
		case ports.EvProcedurePlanRequested:
			out, err = d.Planner.HandleProcedurePlanRequested(ctx, evt)
		case ports.EvLLMPlanRequested:
			out, err = d.Planner.HandleLLMPlanRequested(ctx, evt)
		case ports.EvPlanProgressed:
			out, err = d.Executor.PlanExec.HandlePlanProgressed(ctx, evt)
		case ports.EvStepCapabilityRetry:
			out, err = d.Executor.PlanExec.HandleStepCapabilityRetry(ctx, evt)
		case ports.EvToolCall:
			out, err = d.Executor.ToolExec.HandleToolCall(ctx, evt)
		case ports.EvObservationReady:
			out, err = d.Judge.StepCritic.HandleObservationReady(ctx, evt)
		case ports.EvTrajectoryCheck:
			out, err = d.Judge.TrajectoryCritic.HandleTrajectoryCheck(ctx, evt)
		case ports.EvTurnFinalized:
			if err := d.Judge.Learner.RecordProcedureLearning(ctx, evt); err != nil {
				log.Printf("engine.Dispatcher.Run: record procedure learning: %v", err)
			}
			if err := d.Judge.RecallArchiver.ArchiveRecall(ctx, evt); err != nil {
				log.Printf("engine.Dispatcher.Run: archive recall: %v", err)
			}
			out = nil
		default:
			return nil
		}
		if err != nil {
			log.Printf("engine.Dispatcher.Run: abort event=%s iter=%d err=%v", evt.Type, n, err)
			return err
		}
		if out == nil {
			return nil
		}
		evt = *out
	}
	return nil
}
