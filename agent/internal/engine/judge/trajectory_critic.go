package judge

import (
	"context"
	"fmt"
	"strings"

	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
	routinggraph "github.com/OctoSucker/agent/repo/routing_graph"
	"github.com/OctoSucker/agent/repo/task"
)

const maxReplansPerTurn = 5

type TrajectoryCritic struct {
	Tasks         *task.TaskStore
	RouteGraph    *routinggraph.RoutingGraph
	TrajectoryLLM *llmclient.OpenAI
}

func NewTrajectoryCritic(tasks *task.TaskStore, routeGraph *routinggraph.RoutingGraph, trajectoryLLM *llmclient.OpenAI) *TrajectoryCritic {
	return &TrajectoryCritic{Tasks: tasks, RouteGraph: routeGraph, TrajectoryLLM: trajectoryLLM}
}

func (c *TrajectoryCritic) HandleTrajectoryCheck(ctx context.Context, pl ports.PayloadTrajectoryCheck) (*ports.Event, error) {
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

	if !verdict.GoalMet {
		if task.ReplanCount >= maxReplansPerTurn {
			return nil, fmt.Errorf("trajectory_critic: goal not met after %d replans", maxReplansPerTurn)
		}
		if err := task.TruncatePlanFromStep(""); err != nil {
			return nil, fmt.Errorf("trajectory_critic: truncate for replan: %w", err)
		}
		task.Reply = ""
		task.TrajectorySummary = verdict.Rationale
		task.ReplanCount++
		if err := c.Tasks.Put(task); err != nil {
			return nil, err
		}
		return ports.EventPtr(ports.Event{Type: ports.EvUserInput, Payload: ports.PayloadUserInput{
			TaskID: pl.TaskID,
			Text:   task.UserInput,
		}}), nil
	} else {
		reply, err := ports.UserReplyFromPlan(task.Plan)
		if err != nil {
			return nil, fmt.Errorf("trajectory_critic: user reply from task.Plan: %w", err)
		}
		task.Reply = reply
		task.TrajectorySummary = fmt.Sprintf("计划 %d 步已全部执行完成。", len(task.Plan.Steps)) + "\n---\n" + verdict.Rationale
		task.ReplanCount = 0
		if err := c.RouteGraph.IncTotalRunsAndPersist(); err != nil {
			return nil, fmt.Errorf("trajectory_critic: total runs: %w", err)
		}
		if err := c.Tasks.Put(task); err != nil {
			return nil, err
		}
		return nil, nil
	}
}

func buildTrajectoryJudgePrompt(userRequest string, plan *ports.Plan) (string, string, error) {

	const trajSystemJSON = `
	You are a trajectory judge for an AI agent.
	
	Your job is to determine whether the completed multi-step execution actually satisfied the user's request.
	
	Important:
	All tools ran without errors, but the task may still have failed logically.
	You must judge whether the user's request was actually fulfilled.
	
	--------------------------------------------------
	EVALUATION RULES
	
	Judge based on:
	- The user request
	- Each step goal
	- Each tool output
	
	Focus on the FINAL RESULT, not whether the steps executed successfully.
	
	The goal is met ONLY IF:
	- The user's request is fully completed
	- The final outputs contain the requested result or information
	- The result is relevant and complete
	
	The goal is NOT met IF:
	- The result is incomplete
	- The result is unrelated
	- The tools ran but did not produce the requested result
	- The agent stopped too early
	- The outputs are empty or useless
	- The user asked a question but no answer was produced
	
	Be strict.
	
	--------------------------------------------------
	OUTPUT FORMAT
	
	You must respond with JSON only.
	Do NOT include markdown.
	Do NOT include any text outside JSON.
	Do NOT include extra fields.
	
	Return exactly this JSON format:
	
	{
	  "goal_met": true or false,
	  "rationale": "2-4 sentences explaining why the request was or was not satisfied."
	}
	
	--------------------------------------------------
	SELF CHECK BEFORE OUTPUT
	
	- Did the final result actually satisfy the user request?
	- Is the task fully completed?
	- If unsure, return goal_met = false.
	`

	var b strings.Builder
	fmt.Fprintf(&b, "USER REQUEST:\n%s\n\nEXECUTION TRAJECTORY:\n\n", userRequest)

	for _, st := range plan.Steps {
		out := st.PrimaryText()
		fmt.Fprintf(&b,
			"Step ID: %s\nGoal: %s\nTool: %s\nTool Output:\n%s\n----------------------\n",
			st.ID, st.Goal, st.Node.String(), out)
	}
	return trajSystemJSON, b.String(), nil
}

type trajectoryVerdict struct {
	GoalMet   bool   `json:"goal_met"`
	Rationale string `json:"rationale"`
}
