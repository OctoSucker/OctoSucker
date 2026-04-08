package judge

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/OctoSucker/octosucker/engine/types"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
	"github.com/OctoSucker/octosucker/repo/routegraph"
	"github.com/OctoSucker/octosucker/repo/taskstore"
)

const maxReplansPerTurn = 5

// Trajectory outcomes returned by the judge LLM (JSON field "outcome").
const (
	outcomeComplete = "complete" // user request satisfied; end turn
	outcomeContinue = "continue" // trajectory healthy; plan one more step
	outcomeAbort    = "abort"    // cannot satisfy request; end turn with rationale
	outcomeReplan   = "replan"   // discard bad suffix and replan; optional truncate_from_step_id
)

type TrajectoryCritic struct {
	Tasks         *taskstore.TaskStore
	RouteGraph    *routegraph.Graph
	TrajectoryLLM *llmclient.OpenAI
}

func NewTrajectoryCritic(tasks *taskstore.TaskStore, routeGraph *routegraph.Graph, trajectoryLLM *llmclient.OpenAI) *TrajectoryCritic {
	return &TrajectoryCritic{Tasks: tasks, RouteGraph: routeGraph, TrajectoryLLM: trajectoryLLM}
}

func (c *TrajectoryCritic) HandleTrajectoryCheck(ctx context.Context, pl types.PayloadTrajectoryCheck) (*types.Event, error) {
	task, ok := c.Tasks.Get(pl.TaskID)
	if !ok {
		return nil, fmt.Errorf("trajectory_critic: task %q not found", pl.TaskID)
	}

	systemPrompt, userPrompt, err := buildTrajectoryJudgePrompt(task.UserInput, task.Plan)
	if err != nil {
		return nil, err
	}
	var verdict trajectoryVerdict
	if err := c.TrajectoryLLM.CompleteJSON(ctx, systemPrompt, userPrompt, &verdict); err != nil {
		return nil, fmt.Errorf("trajectory_critic: verdict: %w", err)
	}

	outcome, err := normalizeTrajectoryOutcome(verdict.Outcome)
	if err != nil {
		return nil, fmt.Errorf("trajectory_critic: %w", err)
	}

	switch outcome {
	case outcomeComplete:
		if task.Plan == nil || len(task.Plan.Steps) == 0 {
			return nil, fmt.Errorf("trajectory_critic: outcome complete but plan has no steps")
		}
		reply, err := types.UserReplyFromPlan(task.Plan)
		if err != nil {
			return nil, fmt.Errorf("trajectory_critic: user reply from task.Plan: %w", err)
		}
		task.Reply = reply
		task.TrajectorySummary = fmt.Sprintf("计划 %d 步已全部执行完成。", len(task.Plan.Steps)) + "\n---\n" + verdict.Rationale
		task.ReplanCount = 0
		if err := c.Tasks.Put(task); err != nil {
			return nil, err
		}
		logTrajectoryOutcome(pl.TaskID, outcome, task, verdict, nil)
		return nil, nil

	case outcomeAbort:
		task.Reply = verdict.Rationale
		task.TrajectorySummary = ""
		task.ReplanCount = 0
		if err := c.Tasks.Put(task); err != nil {
			return nil, err
		}
		logTrajectoryOutcome(pl.TaskID, outcome, task, verdict, nil)
		return nil, nil

	case outcomeContinue:
		task.Reply = ""
		task.TrajectorySummary = verdict.Rationale
		task.ReplanCount = 0
		if err := c.Tasks.Put(task); err != nil {
			return nil, err
		}
		next := types.EventPtr(types.Event{Type: types.EvUserInput, Payload: types.PayloadUserInput{
			TaskID: pl.TaskID,
			Text:   task.UserInput,
		}})
		logTrajectoryOutcome(pl.TaskID, outcome, task, verdict, next)
		return next, nil

	case outcomeReplan:
		if task.ReplanCount >= maxReplansPerTurn {
			log.Printf("trajectory_critic: task=%s outcome=%s blocked max_replans=%d", pl.TaskID, outcome, maxReplansPerTurn)
			return nil, fmt.Errorf("trajectory_critic: goal not met after %d replans", maxReplansPerTurn)
		}
		truncateID := strings.TrimSpace(verdict.TruncateFromStepID)
		if truncateID != "" {
			if task.Plan == nil || task.Plan.FindStep(truncateID) == nil {
				return nil, fmt.Errorf("trajectory_critic: truncate_from_step_id %q not in plan", truncateID)
			}
			if err := task.TruncatePlanFromStep(truncateID); err != nil {
				return nil, fmt.Errorf("trajectory_critic: truncate for replan: %w", err)
			}
		} else {
			if err := task.TruncatePlanFromStep(""); err != nil {
				return nil, fmt.Errorf("trajectory_critic: truncate for replan: %w", err)
			}
		}
		task.Reply = ""
		task.TrajectorySummary = verdict.Rationale
		task.ReplanCount++
		if err := c.Tasks.Put(task); err != nil {
			return nil, err
		}
		next := types.EventPtr(types.Event{Type: types.EvUserInput, Payload: types.PayloadUserInput{
			TaskID: pl.TaskID,
			Text:   task.UserInput,
		}})
		logTrajectoryOutcome(pl.TaskID, outcome, task, verdict, next)
		return next, nil

	default:
		return nil, fmt.Errorf("trajectory_critic: internal outcome %q", outcome)
	}
}

func logTrajectoryOutcome(taskID, outcome string, task *types.Task, v trajectoryVerdict, next *types.Event) {
	nextTyp := "nil"
	if next != nil {
		nextTyp = next.Type
	}
	nSteps, lastTool, lastStatus := 0, "", ""
	if task.Plan != nil {
		nSteps = len(task.Plan.Steps)
		if nSteps > 0 {
			s := task.Plan.Steps[nSteps-1]
			lastTool = s.Node.Tool
			lastStatus = s.Status
		}
	}
	log.Printf(
		"trajectory_critic: task=%s outcome=%s next_evt=%s plan_steps=%d last_tool=%s last_status=%s replan_count=%d truncate=%q rationale=%s",
		taskID, outcome, nextTyp, nSteps, lastTool, lastStatus, task.ReplanCount, strings.TrimSpace(v.TruncateFromStepID), clipForLog(v.Rationale, 400),
	)
}

func clipForLog(s string, maxRunes int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= maxRunes {
		return string(r)
	}
	return string(r[:maxRunes]) + "…"
}

func normalizeTrajectoryOutcome(s string) (string, error) {
	o := strings.ToLower(strings.TrimSpace(s))
	switch o {
	case outcomeComplete, outcomeContinue, outcomeAbort, outcomeReplan:
		return o, nil
	default:
		return "", fmt.Errorf("unknown outcome %q (want %q, %q, %q, or %q)",
			s, outcomeComplete, outcomeContinue, outcomeAbort, outcomeReplan)
	}
}

func buildTrajectoryJudgePrompt(userRequest string, plan *types.Plan) (string, string, error) {

	const trajSystemJSON = `
	You are a trajectory judge for an AI agent that plans one executable step at a time.

	Step-level success (tool ran, schema-valid arguments) is handled elsewhere. Your job is trajectory-level only.

	Trust the tool output when the step succeeded: if the trajectory already ran the appropriate tool and the JSON/text output is valid (including empty arrays or empty lists such as "relations": []), that is a real answer from the pipeline, not a bug to fix by replanning. Do not choose "replan" merely because you believe the source text "should" have yielded more rows than the tool returned.

	--------------------------------------------------
	OUTCOME (exactly one string)

	Choose exactly one value for the JSON field "outcome":

	- "complete" — The USER REQUEST is addressed: either the goal is fully met, or the agent already executed the right kind of step and produced a legitimate final result. That includes extraction/summarization/classification tasks where the tool succeeded and returned a valid structured result even when that result is empty (e.g. no relations, no matches). In those cases prefer "complete" over "replan": the user gets the tool's outcome; state in "rationale" that nothing was extracted if helpful.
	- "continue" — Not done yet, but the trajectory is coherent forward progress (including valid discovery before effect tools); the planner should add the next step.
	- "abort" — Stop: the request cannot be satisfied in this agent (ambiguous, impossible, out of scope, contradictory, or no legitimate work left that counts as success). Put a user-facing explanation in "rationale".
	- "replan" — The executed steps are off course: wrong tools for the goal, missing prerequisite discovery, clearly wrong arguments, or a dead-end strategy. The plan will be trimmed and the planner will run again. Do NOT use "replan" only because a successful extraction step returned an empty list. If a suffix starting at a specific listed step should be removed, set "truncate_from_step_id" to that Step ID (copy from "Step ID:" lines). If the whole current plan should be discarded, use "" for truncate_from_step_id.

	--------------------------------------------------
	OUTPUT FORMAT

	You must respond with JSON only.
	Do NOT include markdown.
	Do NOT include any text outside JSON.
	Do NOT include extra fields.

	Return exactly this JSON shape:

	{
	  "outcome": "complete" | "continue" | "abort" | "replan",
	  "truncate_from_step_id": "",
	  "rationale": "2-4 sentences."
	}

	Use "truncate_from_step_id" only when outcome is "replan"; otherwise use "".
	`

	var b strings.Builder
	fmt.Fprintf(&b, "USER REQUEST:\n%s\n\nEXECUTION TRAJECTORY:\n\n", userRequest)

	if plan == nil || len(plan.Steps) == 0 {
		fmt.Fprintf(&b, "(no steps)\n")
	} else {
		for _, st := range plan.Steps {
			fmt.Fprintf(&b,
				"Step ID: %s\nGoal: %s\nTool: %s\nTool Output:\n%s\n----------------------\n",
				st.ID, st.Goal, st.Node.String(), st.PrimaryText())
		}
	}
	return trajSystemJSON, b.String(), nil
}

type trajectoryVerdict struct {
	Outcome            string `json:"outcome"`
	TruncateFromStepID string `json:"truncate_from_step_id"`
	Rationale          string `json:"rationale"`
}
