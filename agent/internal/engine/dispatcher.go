// Package engine wires the event loop: Planner (route + plan materialization), PlanExec (steps), ToolExec, Judge.
package engine

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/internal/engine/execution"
	judgepkg "github.com/OctoSucker/agent/internal/engine/judge"
	"github.com/OctoSucker/agent/internal/engine/planning"
	"github.com/OctoSucker/agent/internal/store/capability"
	skillsbuiltin "github.com/OctoSucker/agent/internal/store/capability/builtin/skills"
	"github.com/OctoSucker/agent/internal/store/nodefailure"
	procedure "github.com/OctoSucker/agent/internal/store/procedure"
	"github.com/OctoSucker/agent/internal/store/recall"
	routinggraph "github.com/OctoSucker/agent/internal/store/routing_graph"
	"github.com/OctoSucker/agent/internal/store/task"
	"github.com/OctoSucker/agent/pkg/llmclient"
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
	mcpEndpoints []string,
	openai config.OpenAI,
	execCfg config.Exec,
	telegramCfg config.Telegram,
	skillsDir string,
	sqlDB *sql.DB,
) (*Dispatcher, error) {
	taskStore, err := task.NewTaskStore(sqlDB)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: task store: %w", err)
	}

	plannerLLM := llmclient.NewOpenAI(openai.BaseURL, openai.APIKey, openai.Model, openai.EmbeddingModel)
	embedder := llmclient.NewOpenAI(openai.BaseURL, openai.APIKey, openai.Model, openai.EmbeddingModel)
	trajectoryLLM := llmclient.NewOpenAI(openai.BaseURL, openai.APIKey, openai.Model, openai.EmbeddingModel)
	skillStore, err := skillsbuiltin.NewFromDir(ctx, skillsDir, plannerLLM)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: skills store: %w", err)
	}
	capReg, err := capability.NewCapabilityRegistry(ctx, mcpEndpoints, execCfg, telegramCfg, skillStore, plannerLLM)
	if err != nil {
		return nil, err
	}
	if err := skillStore.Reload(ctx, plannerLLM); err != nil {
		return nil, fmt.Errorf("dispatcher: skills reload with capability catalog: %w", err)
	}
	if err := capReg.ResyncToolsFromRunners(ctx); err != nil {
		return nil, fmt.Errorf("dispatcher: resync tools after skills reload: %w", err)
	}
	routeGraph, err := routinggraph.NewRoutingGraphFromCapabilityRegistry(capReg, sqlDB)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: routing graph: %w", err)
	}
	nodeFailures, err := nodefailure.NewNodeFailureStats(sqlDB)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: node failures: %w", err)
	}
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
			skillStore,
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
	}
	execAgent := execution.NewAgentExecutor(
		taskStore,
		routeGraph,
		capReg,
	)
	execAgent.ToolExec.OnCatalogChanged = func(syncCtx context.Context) error {
		if err := capReg.ResyncToolsFromRunners(syncCtx); err != nil {
			return err
		}
		routeGraph.ReplaceStaticFromCapabilities(capReg.AllCapabilities())
		return nil
	}
	d.Executor = execAgent

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
		case ports.EvPlanProgressed:
			out, err = d.Executor.PlanExec.HandlePlanProgressed(ctx, evt)
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
