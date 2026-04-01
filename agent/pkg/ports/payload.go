package ports

import "github.com/OctoSucker/agent/repo/graph"

type PayloadUserInput struct {
	TaskID string `json:"task_id"`
	Text   string `json:"text"`
}

type PayloadPlanProgressed struct {
	TaskID string `json:"task_id"`
}

type PayloadToolCall struct {
	TaskID    string         `json:"task_id"`
	StepID    string         `json:"step_id"`
	Node      graph.Node     `json:"node"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type PayloadObservation struct {
	TaskID string     `json:"task_id"`
	StepID string     `json:"step_id"`
	Result ToolResult `json:"result"`
}

type PayloadTrajectoryCheck struct {
	TaskID string `json:"task_id"`
}
