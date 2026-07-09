package judge

import (
	"context"
	"fmt"
	"log"
	"strings"

	types "github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
)

type trajectoryVerdict struct {
	Outcome                 string `json:"outcome"`
	NextPhase               string `json:"next_phase"`
	LastStepLearningOutcome string `json:"last_step_learning_outcome"`
	TruncateFromStepID      string `json:"truncate_from_step_id"`
	Rationale               string `json:"rationale"`
}

type TrajectoryEvaluatorJudge struct {
	LLM        *llmclient.OpenAI
	ProjectCtx string
}

func (j *TrajectoryEvaluatorJudge) Evaluate(ctx context.Context, task *types.Task) (trajectoryVerdict, error) {
	if j == nil || j.LLM == nil {
		return trajectoryVerdict{}, fmt.Errorf("trajectory judge: llm is required")
	}
	if task == nil {
		return trajectoryVerdict{}, fmt.Errorf("trajectory judge: task is nil")
	}
	systemPrompt, userPrompt, err := buildTrajectoryJudgePrompt(task.UserInput, task.Phase, task.EvidenceSummary, j.ProjectCtx, task.Plan)
	if err != nil {
		return trajectoryVerdict{}, err
	}
	var verdict trajectoryVerdict
	if err := j.LLM.CompleteJSON(ctx, systemPrompt, userPrompt, &verdict); err != nil {
		return trajectoryVerdict{}, err
	}
	outcome, err := normalizeTrajectoryOutcome(verdict.Outcome)
	if err != nil {
		return trajectoryVerdict{}, err
	}
	verdict.Outcome = outcome
	nextPhase, err := normalizeNextPhase(verdict.NextPhase, outcome)
	if err != nil {
		return trajectoryVerdict{}, err
	}
	verdict.NextPhase = nextPhase
	learningOutcome, err := normalizeLastStepLearningOutcome(verdict.LastStepLearningOutcome, outcome)
	if err != nil {
		return trajectoryVerdict{}, err
	}
	verdict.LastStepLearningOutcome = learningOutcome
	return verdict, nil
}

const (
	lastStepLearningSuccess = "success"
	lastStepLearningNeutral = "neutral"
	lastStepLearningFailure = "failure"
)

func (v trajectoryVerdict) LastStepLearningSuccess() (bool, bool) {
	switch v.LastStepLearningOutcome {
	case lastStepLearningSuccess:
		return true, true
	case lastStepLearningFailure:
		return false, true
	default:
		return false, false
	}
}

type TrajectoryDecisionApplier struct{}

func (a *TrajectoryDecisionApplier) Apply(taskID string, task *types.Task, verdict trajectoryVerdict, finalAnswer string) (*types.Event, error) {
	switch verdict.Outcome {
	case outcomeComplete:
		if task.Plan == nil || !task.Plan.HasSteps() {
			return nil, fmt.Errorf("outcome complete but plan has no steps")
		}
		reply := strings.TrimSpace(finalAnswer)
		if reply == "" {
			var err error
			reply, err = task.Plan.UserReply()
			if err != nil {
				return nil, fmt.Errorf("user reply from plan: %w", err)
			}
		}
		if err := task.MarkCompleted(reply, verdict.Rationale); err != nil {
			return nil, err
		}
		return nil, nil

	case outcomeAbort:
		reply := strings.TrimSpace(finalAnswer)
		if reply == "" {
			reply = verdict.Rationale
		}
		if err := task.MarkAborted(reply); err != nil {
			return nil, err
		}
		return nil, nil

	case outcomeContinue:
		if err := task.MarkContinuing(verdict.Rationale); err != nil {
			return nil, err
		}
		if err := task.SetPhase(types.TaskPhase(verdict.NextPhase)); err != nil {
			return nil, err
		}
		return nextUserInputEvent(taskID, task.UserInput), nil

	case outcomeReplan:
		if task.ReplanCount >= maxReplansPerTurn {
			log.Printf("trajectory_critic: task=%s outcome=%s blocked max_replans=%d", taskID, verdict.Outcome, maxReplansPerTurn)
			return nil, fmt.Errorf("goal not met after %d replans", maxReplansPerTurn)
		}
		truncateID := strings.TrimSpace(verdict.TruncateFromStepID)
		if truncateID != "" {
			if task.Plan == nil || task.Plan.FindStep(truncateID) == nil {
				return nil, fmt.Errorf("truncate_from_step_id %q not in plan", truncateID)
			}
			if err := task.TruncatePlanFromStep(truncateID); err != nil {
				return nil, fmt.Errorf("truncate for replan: %w", err)
			}
		} else {
			if err := task.TruncatePlanFromStep(""); err != nil {
				return nil, fmt.Errorf("truncate for replan: %w", err)
			}
		}
		if err := task.MarkReplanning(verdict.Rationale); err != nil {
			return nil, err
		}
		if verdict.NextPhase != "" {
			if err := task.SetPhase(types.TaskPhase(verdict.NextPhase)); err != nil {
				return nil, err
			}
		}
		return nextUserInputEvent(taskID, task.UserInput), nil

	default:
		return nil, fmt.Errorf("internal outcome %q", verdict.Outcome)
	}
}

func nextUserInputEvent(taskID, text string) *types.Event {
	return types.EventPtr(types.Event{
		Type: types.EvTurnRequested,
		Payload: types.TurnRequest{
			TaskID: taskID,
			Text:   text,
		},
	})
}

func buildTrajectoryJudgePrompt(userRequest string, phase types.TaskPhase, evidenceSummary string, projectCtx string, plan *types.Plan) (string, string, error) {
	const trajSystemJSON = `
	You are a trajectory judge for an AI agent that plans one executable step at a time.

	Step-level success (tool ran, schema-valid arguments) is handled elsewhere. Your job is trajectory-level only.

	Trust the tool output and Tool result meta when the step succeeded: if the trajectory already ran the appropriate tool and the JSON/text output is valid (including empty arrays or empty lists such as "relations": [] with count=0 empty=true), that is a real answer from the pipeline, not a bug to fix by replanning. Do not choose "replan" merely because you believe the source text "should" have yielded more rows than the tool returned.

	If CURRENT TASK PHASE is "synthesis" and the executed observations are enough to answer the user, choose "complete"; the separate final-answer synthesizer will write the user-facing response. Do not choose "continue" just to ask another natural-language tool to rewrite the same observations.

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
	  "next_phase": "discovery" | "execution" | "synthesis" | "done" | "abort",
	  "last_step_learning_outcome": "success" | "neutral" | "failure",
	  "truncate_from_step_id": "",
	  "rationale": "2-4 sentences."
	}

	"next_phase" is the state the planner should use if another step is needed:
	- "discovery" when more capability lookup, skill reading, or tool-list inspection is needed.
	- "execution" when the next step should perform real work with a concrete tool.
	- "synthesis" when enough observations exist and the remaining work is only writing the final answer.
	- "done" only with outcome "complete".
	- "abort" only with outcome "abort".

	"last_step_learning_outcome" is the semantic learning signal for the LAST executed step only:
	- "success" when the last step's tool output materially helped satisfy the user request or provided a necessary prerequisite for the next coherent step.
	- "failure" when the last step executed but was semantically off-course, used the wrong tool/arguments, or produced output that does not help the user request and should teach routing to avoid that transition.
	- "neutral" when the last step output is valid but not useful as a routing lesson: legitimate empty/no-match results, user-facing aborts, insufficient information, or cases where no executed step should be rewarded or punished.
	For "complete" and "continue", prefer "success" only if the last step actually helped. For "replan", prefer "failure" when the last step is part of the bad strategy; use "neutral" if the replan is caused by ambiguity or missing external information rather than a bad tool choice.

	Use "truncate_from_step_id" only when outcome is "replan"; otherwise use "".
	`

	evidence := strings.TrimSpace(evidenceSummary)
	if evidence == "" {
		evidence = "(none)"
	}
	project := strings.TrimSpace(projectCtx)
	if project == "" {
		project = "(none)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "USER REQUEST:\n%s\n\nPROJECT INSTRUCTIONS:\n%s\n\nCURRENT TASK PHASE:\n%s\n\nACCUMULATED EVIDENCE:\n%s\n\nEXECUTION TRAJECTORY:\n\n", userRequest, project, phase, evidence)
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
