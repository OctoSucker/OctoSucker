// Package engine wires the event loop: Planner (route + plan materialization), PlanExec (step + tool invoke), Judge.
package engine

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/OctoSucker/octosucker/config"
	"github.com/OctoSucker/octosucker/engine/execution"
	judgepkg "github.com/OctoSucker/octosucker/engine/judge"
	"github.com/OctoSucker/octosucker/engine/planning"
	"github.com/OctoSucker/octosucker/engine/types"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
	"github.com/OctoSucker/octosucker/repo/routegraph"
	"github.com/OctoSucker/octosucker/repo/taskstore"
	"github.com/OctoSucker/octosucker/repo/toolprovider"
	"github.com/OctoSucker/octosucker/store"
)

const MaxSteps = 200

type Dispatcher struct {
	Planner  *planning.Planner
	Judge    *judgepkg.Judge
	PlanExec *execution.PlanExecutor
}

func NewDispatcher(
	ctx context.Context,
	mcpEndpoints []string,
	openai config.OpenAI,
	execCfg config.Exec,
	telegramCfg config.Telegram,
	skillsDir string,
	data *store.DB,
) (*Dispatcher, error) {
	taskStore := taskstore.NewTaskStore()

	plannerLLM := llmclient.NewOpenAI(openai.BaseURL, openai.APIKey, openai.Model, openai.EmbeddingModel)
	trajectoryLLM := llmclient.NewOpenAI(openai.BaseURL, openai.APIKey, openai.Model, openai.EmbeddingModel)
	toolRegistry, err := toolprovider.NewRegistry(ctx, mcpEndpoints, execCfg, telegramCfg, skillsDir, plannerLLM)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: tool registry: %w", err)
	}

	routeGraph, err := routegraph.New(data, toolRegistry.AllToolIDs())
	if err != nil {
		return nil, fmt.Errorf("dispatcher: route graph: %w", err)
	}

	planner, err := planning.NewPlanner(
		taskStore,
		toolRegistry,
		routeGraph,
		plannerLLM,
	)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: planner: %w", err)
	}

	return &Dispatcher{
		Planner: planner,
		Judge: judgepkg.NewJudge(
			taskStore,
			routeGraph,
			trajectoryLLM,
		),
		PlanExec: &execution.PlanExecutor{
			Tasks:        taskStore,
			ToolRegistry: toolRegistry,
		},
	}, nil
}

func (d *Dispatcher) Run(ctx context.Context, start types.Event) error {
	evt := start
	for n := 1; n <= MaxSteps; n++ {
		var (
			out *types.Event
			err error
		)
		switch evt.Type {
		case types.EvUserInput:
			pl, ok := evt.Payload.(types.PayloadUserInput)
			if !ok {
				return fmt.Errorf("dispatcher: invalid payload for %s", types.EvUserInput)
			}
			out, err = d.Planner.HandleUserInput(ctx, pl)
		case types.EvPlanProgressed:
			pl, ok := evt.Payload.(types.PayloadPlanProgressed)
			if !ok {
				return fmt.Errorf("dispatcher: invalid payload for %s", types.EvPlanProgressed)
			}
			out, err = d.PlanExec.HandlePlanProgressed(ctx, pl)
		case types.EvObservationReady:
			pl, ok := evt.Payload.(types.PayloadObservation)
			if !ok {
				return fmt.Errorf("dispatcher: invalid payload for %s", types.EvObservationReady)
			}
			out, err = d.Judge.StepCritic.HandleObservationReady(ctx, pl)
		case types.EvTrajectoryCheck:
			pl, ok := evt.Payload.(types.PayloadTrajectoryCheck)
			if !ok {
				return fmt.Errorf("dispatcher: invalid payload for %s", types.EvTrajectoryCheck)
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
			log.Printf("dispatcher: iter=%d out=nil (turn end)", n)
			return nil
		}
		evt = *out
	}
	return d.persistEmptyTurnIfNeeded(evt)
}

func (d *Dispatcher) persistEmptyTurnIfNeeded(nextEvt types.Event) error {
	tid, ok := types.TaskIDFromEvent(nextEvt)
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
