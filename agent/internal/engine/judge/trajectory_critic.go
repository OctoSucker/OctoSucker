package judge

import (
	"context"
	"fmt"
	"strings"

	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/OctoSucker/agent/repo/recall"
	routinggraph "github.com/OctoSucker/agent/repo/routing_graph"
	"github.com/OctoSucker/agent/repo/task"
)

// trajSystemJSON asks for a machine-readable verdict; CompleteJSON parses it.
const trajSystemJSON = `You judge whether a finished multi-step tool run actually satisfied the user's request.
All steps completed without tool-level errors. Use the user's message plus each step's goal and tool output.
Respond with JSON only (no markdown fences), exactly this shape:
{"goal_met":true|false,"rationale":"2-4 sentences: why the goal is or is not met; if not, what was missing or wrong."}
Be strict: incomplete, off-topic, or non-answers mean goal_met false.`

type trajectoryVerdict struct {
	GoalMet   bool   `json:"goal_met"`
	Rationale string `json:"rationale"`
}

type TrajectoryCritic struct {
	Tasks         *task.TaskStore
	RouteGraph    *routinggraph.RoutingGraph
	Recall        *recall.RecallCorpus
	TrajectoryLLM *llmclient.OpenAI
}

func NewTrajectoryCritic(tasks *task.TaskStore, routeGraph *routinggraph.RoutingGraph, recallCorpus *recall.RecallCorpus, trajectoryLLM *llmclient.OpenAI) *TrajectoryCritic {
	return &TrajectoryCritic{Tasks: tasks, RouteGraph: routeGraph, Recall: recallCorpus, TrajectoryLLM: trajectoryLLM}
}

func (c *TrajectoryCritic) HandleTrajectoryCheck(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadTrajectoryCheck)
	task, ok := c.Tasks.Get(pl.TaskID)
	if !ok {
		return nil, fmt.Errorf("trajectory_critic: task %q not found", pl.TaskID)
	}
	plan := task.Plan
	if plan == nil || len(plan.Steps) == 0 {
		return nil, fmt.Errorf("trajectory_critic: invariant: TrajectoryCheck with empty plan")
	}
	if task.RouteSnap == nil {
		return nil, fmt.Errorf("trajectory_critic: nil RouteSnap")
	}
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "User request:\n%s\n\nExecuted steps:\n", task.UserInput.Text)
	for _, st := range plan.Steps {
		if st.Status != "done" {
			return nil, fmt.Errorf("trajectory_critic: invariant: step %q not done", st.ID)
		}
		fmt.Fprintf(&prompt, "StepID: %s, Goal: %s, Capability: %s, Tool: %s, Output: %s\n", st.ID, st.Goal, st.Capability, st.Tool, st.PrimaryText())
	}

	var verdict trajectoryVerdict
	if err := c.TrajectoryLLM.CompleteJSON(ctx, trajSystemJSON, prompt.String(), &verdict); err != nil {
		return nil, fmt.Errorf("trajectory_critic: verdict: %w", err)
	}
	if strings.TrimSpace(verdict.Rationale) == "" {
		return nil, fmt.Errorf("trajectory_critic: empty rationale")
	}

	if !verdict.GoalMet {
		if task.ReplanCount >= maxReplansPerTurn {
			return nil, fmt.Errorf("trajectory_critic: goal not met after %d replans", maxReplansPerTurn)
		}
		task.Reply = ""
		task.TrajectorySummary = verdict.Rationale
		task.ReplanCount++
		if err := task.TruncatePlanFromStep(""); err != nil {
			return nil, fmt.Errorf("trajectory_critic: truncate for replan: %w", err)
		}
		if err := c.Tasks.Put(task); err != nil {
			return nil, err
		}
		return ports.EventPtr(ports.Event{Type: ports.EvUserInput, Payload: ports.PayloadUserInput{
			TaskID:              pl.TaskID,
			Text:                task.UserInput.Text,
			PlannerContinuation: true,
			TelegramChatID:      task.UserInput.TelegramChatID,
		}}), nil
	}

	task.Reply = ports.UserReplyFromPlan(plan)
	task.TrajectorySummary = fmt.Sprintf("计划 %d 步已全部执行完成。", len(plan.Steps)) + "\n---\n" + verdict.Rationale
	task.ReplanCount = 0
	task.ToolFailureTotal = 0

	if err := c.RouteGraph.IncTotalRunsAndPersist(); err != nil {
		return nil, fmt.Errorf("trajectory_critic: total runs: %w", err)
	}
	if doc := task.RecallPlannerCorpusDocument(plan); doc != "" {
		if err := c.Recall.Write(ctx, doc); err != nil {
			return nil, fmt.Errorf("trajectory_critic: recall write: %w", err)
		}
	}

	if err := c.Tasks.Put(task); err != nil {
		return nil, err
	}
	return nil, nil
}
