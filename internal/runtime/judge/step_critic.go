package judge

import (
	"context"
	"fmt"
	"log"
	"strings"

	rt "github.com/OctoSucker/octosucker/internal/runtime/toolrouting"
	types "github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/runtime/taskstore"
)

type StepEvaluator struct {
	Tasks      *taskstore.TaskStore
	RouteGraph TransitionRecorder
}

func NewStepEvaluator(tasks *taskstore.TaskStore, routeGraph TransitionRecorder) *StepEvaluator {
	return &StepEvaluator{Tasks: tasks, RouteGraph: routeGraph}
}

func (x *StepEvaluator) HandleObservationReady(ctx context.Context, pl types.StepObserved) (*types.Event, error) {
	task, ok := x.Tasks.Get(pl.TaskID)
	if !ok {
		return nil, fmt.Errorf("step_critic: task %q not found", pl.TaskID)
	}
	if task.Plan == nil {
		return nil, fmt.Errorf("step_critic: task %q has no plan", pl.TaskID)
	}
	step := task.Plan.FindStep(pl.StepID)
	if step == nil {
		return nil, fmt.Errorf("step_critic: task %q step %q not found", pl.TaskID, pl.StepID)
	}
	currentNode := step.Node
	prevStep := task.Plan.FindPrevStep(pl.StepID)
	prevNode := rt.Node{}
	if prevStep != nil {
		prevNode = prevStep.Node
	}
	if pl.Result.Err != nil {
		step.ToolResult = pl.Result.WithInferredMeta(step.Node.Tool)
		task.AppendEvidenceFromStep(step)
		task.AppendTrace("tool failed tool=%s err=%s", step.Node.Tool, clipForLog(strings.TrimSpace(pl.Result.Err.Error()), 260))
		if err := x.RouteGraph.RecordTransition(
			task.UserInput,
			0, 0,
			prevNode,
			currentNode,
			false,
		); err != nil {
			return nil, fmt.Errorf("step_critic: RecordTransition: %w", err)
		}
		if nonRetryableToolError(pl.Result.Err) {
			reply := fmt.Sprintf(
				"工具 %s 执行失败，错误看起来不可通过重试修复，已停止本轮。最近一次错误：%s",
				step.Node.Tool,
				clipForLog(strings.TrimSpace(pl.Result.Err.Error()), 300),
			)
			if err := task.MarkAborted(reply); err != nil {
				return nil, err
			}
			if err := x.Tasks.Put(task); err != nil {
				return nil, err
			}
			log.Printf("step_critic: task=%s step=%s tool=%s non_retryable -> abort", pl.TaskID, pl.StepID, step.Node.Tool)
			return nil, nil
		}
		if err := task.IncrementReplanCount(); err != nil {
			return nil, err
		}
		if task.ReplanCount >= maxReplansPerTurn {
			reply := fmt.Sprintf(
				"工具 %s 连续失败，已停止本轮以避免反复重试。最近一次错误：%s",
				step.Node.Tool,
				clipForLog(strings.TrimSpace(pl.Result.Err.Error()), 240),
			)
			if err := task.MarkAborted(reply); err != nil {
				return nil, err
			}
			if err := x.Tasks.Put(task); err != nil {
				return nil, err
			}
			log.Printf("step_critic: task=%s step=%s tool=%s max_replans=%d -> abort", pl.TaskID, pl.StepID, step.Node.Tool, maxReplansPerTurn)
			return nil, nil
		}
		if err := task.TruncatePlanFromStep(pl.StepID); err != nil {
			log.Printf("------step_critic:  error: %+v", err)
			return nil, err
		}
		if err := x.Tasks.Put(task); err != nil {
			log.Printf("------step_critic:  error: %+v", err)
			return nil, err
		}
		log.Printf("step_critic: task=%s step=%s tool=%s err -> replan UserInput", pl.TaskID, pl.StepID, step.Node.Tool)
		return types.EventPtr(types.Event{
			Type: types.EvTurnRequested,
			Payload: types.TurnRequest{
				TaskID: pl.TaskID,
				Text:   task.UserInput,
			}},
		), nil
	} else {
		if err := task.MarkStepDone(pl.StepID, pl.Result); err != nil {
			return nil, err
		}
		task.AppendTrace("tool succeeded tool=%s kind=%s count=%d empty=%v", step.Node.Tool, step.ToolResult.Kind, step.ToolResult.Count, step.ToolResult.Empty)
		if deterministicWorkflowShouldContinue(task, step) {
			if err := x.recordDeterministicSuccess(task, pl.StepID); err != nil {
				return nil, err
			}
			if err := task.SetPhase(types.TaskPhaseExecution); err != nil {
				return nil, err
			}
			if err := x.Tasks.Put(task); err != nil {
				return nil, err
			}
			log.Printf("step_critic: task=%s step=%s tool=%s deterministic workflow -> continue", pl.TaskID, pl.StepID, step.Node.Tool)
			return types.EventPtr(types.Event{
				Type: types.EvTurnRequested,
				Payload: types.TurnRequest{
					TaskID: pl.TaskID,
					Text:   task.UserInput,
				},
			}), nil
		}
		if deterministicStepCompletesTurn(step.Node.Tool) {
			answer, ok := deterministicFinalAnswer(task, trajectoryVerdict{Outcome: outcomeComplete})
			if step.Node.Tool == "analyze_us_market_intel" {
				_, message := analysisShouldSend(step)
				if strings.TrimSpace(message) != "" {
					answer, ok = strings.TrimSpace(message), true
				}
			}
			if !ok {
				answer = strings.TrimSpace(step.PrimaryText())
			}
			if answer == "" {
				return nil, fmt.Errorf("step_critic: deterministic completion produced empty answer")
			}
			if err := task.MarkCompleted(answer, "deterministic single-step completion"); err != nil {
				return nil, err
			}
			if err := x.recordDeterministicSuccess(task, pl.StepID); err != nil {
				return nil, err
			}
			if err := x.Tasks.Put(task); err != nil {
				return nil, err
			}
			log.Printf("step_critic: task=%s step=%s tool=%s deterministic -> complete", pl.TaskID, pl.StepID, step.Node.Tool)
			return nil, nil
		}
		if err := task.SetPhase(nextPhaseAfterObservedTool(step.Node.Tool)); err != nil {
			return nil, err
		}
		if err := x.Tasks.Put(task); err != nil {
			return nil, err
		}
		return types.EventPtr(types.Event{
			Type: types.EvTrajectoryEvaluationRequested,
			Payload: types.TrajectoryEvaluationRequest{
				TaskID: pl.TaskID,
			},
		}), nil
	}
}

func nextPhaseAfterObservedTool(tool string) types.TaskPhase {
	switch tool {
	case "list_tool_providers", "list_tools_for_provider", "list_skills", "read_skill", "reload_skills":
		return types.TaskPhaseExecution
	default:
		return types.TaskPhaseSynthesis
	}
}

func deterministicStepCompletesTurn(tool string) bool {
	switch tool {
	case "natural_language_reply", "list_skills", "reload_skills", "list_tool_providers", "list_tools_for_provider":
		return true
	case "analyze_us_market_intel":
		return true
	case "get_skills_root_dir", "list_cronjobs", "run_command":
		return true
	default:
		return false
	}
}

func deterministicWorkflowShouldContinue(task *types.Task, step *types.PlanStep) bool {
	if task == nil || step == nil || !isUSMarketIntelFeishuGoal(task.UserInput) {
		return false
	}
	switch step.Node.Tool {
	case "run_command":
		return isUSMarketScanStep(step)
	case "analyze_us_market_intel":
		shouldSend, _ := analysisShouldSend(step)
		return shouldSend
	default:
		return false
	}
}

func isUSMarketIntelFeishuGoal(input string) bool {
	s := strings.ToLower(strings.TrimSpace(input))
	hasMarket := strings.Contains(s, "美股") || strings.Contains(s, "us market") || strings.Contains(s, "market")
	hasIntel := strings.Contains(s, "情报") || strings.Contains(s, "新闻") || strings.Contains(s, "异动") || strings.Contains(s, "scan") || strings.Contains(s, "report")
	hasFeishu := strings.Contains(s, "飞书") || strings.Contains(s, "feishu") || strings.Contains(s, "lark")
	hasSend := strings.Contains(s, "发送") || strings.Contains(s, "推送") || strings.Contains(s, "send")
	return hasMarket && hasIntel && hasFeishu && hasSend
}

func isUSMarketScanStep(step *types.PlanStep) bool {
	if step == nil || step.Arguments == nil {
		return false
	}
	program, _ := step.Arguments["program"].(string)
	if !strings.Contains(program, "us-market") {
		return false
	}
	rawArgs, ok := step.Arguments["args"].([]any)
	if !ok || len(rawArgs) == 0 {
		return false
	}
	first, _ := rawArgs[0].(string)
	return first == "scan"
}

func analysisShouldSend(step *types.PlanStep) (bool, string) {
	if step == nil {
		return false, ""
	}
	m, ok := step.ToolResult.Output.(map[string]any)
	if !ok {
		return false, ""
	}
	shouldSend, _ := m["should_send"].(bool)
	message, _ := m["message"].(string)
	return shouldSend, strings.TrimSpace(message)
}

func nonRetryableToolError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"argument ",
		"forbidden by blacklist",
		"unknown backend",
		"cannot connect to the docker daemon",
		"no such file or directory",
		"executable file not found",
		"command not found",
		"permission denied",
		"operation not permitted",
		"sandbox_apply",
		"429 too many requests",
		"too many requests",
		"exceeded your current quota",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func (x *StepEvaluator) recordDeterministicSuccess(task *types.Task, stepID string) error {
	if x == nil || x.RouteGraph == nil || task == nil || task.Plan == nil {
		return nil
	}
	step := task.Plan.FindStep(stepID)
	if step == nil {
		return nil
	}
	prev := task.Plan.FindPrevStep(stepID)
	prevNode := rt.Node{}
	if prev != nil {
		prevNode = prev.Node
	}
	return x.RouteGraph.RecordTransition(task.UserInput, 0, 0, prevNode, step.Node, true)
}
