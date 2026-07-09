// Package runtime wires the event loop: Planner, PlanExec, and Judge.
package runtime

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/OctoSucker/octosucker/config"
	"github.com/OctoSucker/octosucker/internal/runtime/execution"
	judgepkg "github.com/OctoSucker/octosucker/internal/runtime/judge"
	types "github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/runtime/planning"
	"github.com/OctoSucker/octosucker/internal/runtime/projectcontext"
	"github.com/OctoSucker/octosucker/internal/runtime/taskstore"
	"github.com/OctoSucker/octosucker/internal/runtime/toolrouting"
	"github.com/OctoSucker/octosucker/internal/storage"
	tools "github.com/OctoSucker/octosucker/internal/tools"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
)

const MaxSteps = 200

type Dispatcher struct {
	ToolRegistry *tools.Registry
	Planner      *planning.Planner
	Evaluator    *judgepkg.Evaluator
	PlanExec     *execution.PlanExecutor
}

type DispatcherDeps struct {
	WorkspaceRoot string
	MCPEndpoints  []string
	OpenAI        config.OpenAI
	Exec          config.Exec
	Telegram      config.Telegram
	SkillsDir     string
	Data          *storage.DB
}

func NewDispatcher(ctx context.Context, deps DispatcherDeps) (*Dispatcher, error) {
	taskStore := taskstore.NewTaskStore()
	projectContext := projectcontext.Load(deps.WorkspaceRoot)

	plannerLLM := llmclient.NewOpenAI(deps.OpenAI.BaseURL, deps.OpenAI.APIKey, deps.OpenAI.Model, deps.OpenAI.EmbeddingModel)
	trajectoryLLM := llmclient.NewOpenAI(deps.OpenAI.BaseURL, deps.OpenAI.APIKey, deps.OpenAI.Model, deps.OpenAI.EmbeddingModel)
	toolRegistry, err := tools.NewRegistry(ctx, tools.RegistryDeps{
		WorkspaceRoot: deps.WorkspaceRoot,
		MCPEndpoints:  deps.MCPEndpoints,
		OpenAI:        deps.OpenAI,
		Exec:          deps.Exec,
		Telegram:      deps.Telegram,
		SkillsDir:     deps.SkillsDir,
		EmbedLLM:      plannerLLM,
		Data:          deps.Data,
	})
	if err != nil {
		return nil, fmt.Errorf("dispatcher: tool registry: %w", err)
	}

	routeGraph, err := toolrouting.New(deps.Data, toolRegistry.AllToolIDs())
	if err != nil {
		return nil, fmt.Errorf("dispatcher: route graph: %w", err)
	}

	planner, err := planning.NewPlanner(
		taskStore,
		toolRegistry,
		routeGraph,
		plannerLLM,
		projectContext,
	)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: planner: %w", err)
	}

	return &Dispatcher{
		ToolRegistry: toolRegistry,
		Planner:      planner,
		Evaluator: judgepkg.NewEvaluator(
			taskStore,
			routeGraph,
			trajectoryLLM,
			projectContext,
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
		case types.EvTurnRequested:
			pl, ok := evt.Payload.(types.TurnRequest)
			if !ok {
				return fmt.Errorf("dispatcher: invalid payload for %s", types.EvTurnRequested)
			}
			out, err = d.Planner.HandleUserInput(ctx, pl)
		case types.EvStepScheduled:
			pl, ok := evt.Payload.(types.StepScheduled)
			if !ok {
				return fmt.Errorf("dispatcher: invalid payload for %s", types.EvStepScheduled)
			}
			out, err = d.PlanExec.HandlePlanProgressed(ctx, pl)
		case types.EvStepObserved:
			pl, ok := evt.Payload.(types.StepObserved)
			if !ok {
				return fmt.Errorf("dispatcher: invalid payload for %s", types.EvStepObserved)
			}
			out, err = d.Evaluator.StepEvaluator.HandleObservationReady(ctx, pl)
		case types.EvTrajectoryEvaluationRequested:
			pl, ok := evt.Payload.(types.TrajectoryEvaluationRequest)
			if !ok {
				return fmt.Errorf("dispatcher: invalid payload for %s", types.EvTrajectoryEvaluationRequested)
			}
			out, err = d.Evaluator.TrajectoryEvaluator.HandleTrajectoryCheck(ctx, pl)
		default:
			return nil
		}
		if err != nil {
			log.Printf("engine.Dispatcher.Run: abort event=%s iter=%d err=%v", evt.Type, n, err)
			return err
		}
		if out == nil {
			d.logTaskDebugSummary(evt)
			log.Printf("dispatcher: iter=%d out=nil (turn end)", n)
			return nil
		}
		evt = *out
	}
	return d.persistEmptyTurnIfNeeded(evt)
}

func (d *Dispatcher) logTaskDebugSummary(evt types.Event) {
	tid, ok := types.TaskIDFromEvent(evt)
	if !ok || d == nil || d.Planner == nil || d.Planner.Tasks == nil {
		return
	}
	task, ok := d.Planner.Tasks.Get(tid)
	if !ok || task == nil {
		return
	}
	plan := "(nil)"
	if task.Plan != nil {
		plan = task.Plan.DebugSummary()
	}
	log.Printf(
		"plan_debug: task=%s phase=%s replan_count=%d reply_empty=%v trajectory=%q plan=%s",
		tid, task.Phase, task.ReplanCount, strings.TrimSpace(task.Reply) == "", clipLog(task.TrajectorySummary, 240), plan,
	)
	if trace := task.TraceSummary(); trace != "" {
		log.Printf("run_trace: task=%s\n%s", tid, trace)
	}
}

func clipLog(s string, maxRunes int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= maxRunes {
		return string(r)
	}
	return string(r[:maxRunes]) + "…"
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
