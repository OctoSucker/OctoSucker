// Package engine wires the event loop: Planner (route + plan materialization), PlanExec (steps), ToolExec, Judge.
package engine

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/internal/engine/execution"
	judgepkg "github.com/OctoSucker/agent/internal/engine/judge"
	"github.com/OctoSucker/agent/internal/engine/planning"
	"github.com/OctoSucker/agent/model"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/OctoSucker/agent/repo/recall"
	routinggraph "github.com/OctoSucker/agent/repo/routing_graph"
	"github.com/OctoSucker/agent/repo/task"
)

const MaxSteps = 200

type Dispatcher struct {
	Planner  *planning.Planner
	Judge    *judgepkg.Judge
	Executor *execution.AgentExecutor
}

func NewDispatcher(
	ctx context.Context,
	mcpEndpoints []string,
	openai config.OpenAI,
	execCfg config.Exec,
	telegramCfg config.Telegram,
	skillsDir string,
	data *model.AgentDB,
) (*Dispatcher, error) {
	taskStore, err := task.NewTaskStore(data)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: task store: %w", err)
	}

	plannerLLM := llmclient.NewOpenAI(openai.BaseURL, openai.APIKey, openai.Model, openai.EmbeddingModel)
	embedder := llmclient.NewOpenAI(openai.BaseURL, openai.APIKey, openai.Model, openai.EmbeddingModel)
	trajectoryLLM := llmclient.NewOpenAI(openai.BaseURL, openai.APIKey, openai.Model, openai.EmbeddingModel)
	routeGraph, err := routinggraph.New(ctx, mcpEndpoints, execCfg, telegramCfg, skillsDir, plannerLLM, data)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: routing graph: %w", err)
	}
	recallCorpus, err := recall.NewRecallCorpus(embedder, data)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: recall corpus: %w", err)
	}

	planner, err := planning.NewPlanner(
		taskStore,
		routeGraph,
		recallCorpus,
		plannerLLM,
	)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: planner: %w", err)
	}

	d := &Dispatcher{
		Planner: planner,
		Judge: judgepkg.NewJudge(
			taskStore,
			routeGraph,
			trajectoryLLM,
			recallCorpus,
		),
	}
	execAgent := execution.NewAgentExecutor(taskStore, routeGraph)
	execAgent.ToolExec.OnCatalogChanged = routeGraph.ResyncToolsAndStaticGraph
	d.Executor = execAgent

	return d, nil
}

func (d *Dispatcher) Run(ctx context.Context, event ports.Event) error {
	evt := event
	for n := 1; n <= MaxSteps; n++ {
		var (
			out *ports.Event
			err error
		)
		switch evt.Type {
		case ports.EvUserInput:
			pl, ok := evt.Payload.(ports.PayloadUserInput)
			if !ok {
				return fmt.Errorf("dispatcher: invalid payload for %s", ports.EvUserInput)
			}
			out, err = d.Planner.HandleUserInput(ctx, pl)
		case ports.EvPlanProgressed:
			pl, ok := evt.Payload.(ports.PayloadPlanProgressed)
			if !ok {
				return fmt.Errorf("dispatcher: invalid payload for %s", ports.EvPlanProgressed)
			}
			out, err = d.Executor.PlanExec.HandlePlanProgressed(ctx, pl)
		case ports.EvToolCall:
			pl, ok := evt.Payload.(ports.PayloadToolCall)
			if !ok {
				return fmt.Errorf("dispatcher: invalid payload for %s", ports.EvToolCall)
			}
			out, err = d.Executor.ToolExec.HandleToolCall(ctx, pl)
		case ports.EvObservationReady:
			pl, ok := evt.Payload.(ports.PayloadObservation)
			if !ok {
				return fmt.Errorf("dispatcher: invalid payload for %s", ports.EvObservationReady)
			}
			out, err = d.Judge.StepCritic.HandleObservationReady(ctx, pl)
		case ports.EvTrajectoryCheck:
			pl, ok := evt.Payload.(ports.PayloadTrajectoryCheck)
			if !ok {
				return fmt.Errorf("dispatcher: invalid payload for %s", ports.EvTrajectoryCheck)
			}
			out, err = d.Judge.TrajectoryCritic.HandleTrajectoryCheck(ctx, pl)
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
	return d.persistEmptyTurnIfNeeded(evt)
}

func (d *Dispatcher) persistEmptyTurnIfNeeded(nextEvt ports.Event) error {
	tid, ok := ports.TaskIDFromEvent(nextEvt)
	if !ok {
		return fmt.Errorf("dispatcher: max steps %d: task id not in pending event type %q", MaxSteps, nextEvt.Type)
	}
	task, ok := d.Planner.Tasks.Get(tid)
	if !ok || task == nil {
		return fmt.Errorf("dispatcher: max steps %d: task %q not found", MaxSteps, tid)
	}
	if strings.TrimSpace(task.Reply) != "" || strings.TrimSpace(task.TrajectorySummary) != "" {
		return nil
	}
	task.Reply = fmt.Sprintf(
		"本轮事件处理已达步数上限（%d），已停止以免长时间空转。若工具多次失败，请检查环境（例如 opencli 是否在 PATH 中；exit 127 通常表示命令未找到）。",
		MaxSteps,
	)
	return d.Planner.Tasks.Put(task)
}
