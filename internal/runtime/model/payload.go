package model

type TurnRequest struct {
	TaskID string `json:"task_id"`
	Text   string `json:"text"`
}

type StepScheduled struct {
	TaskID string `json:"task_id"`
}

type StepObserved struct {
	TaskID string     `json:"task_id"`
	StepID string     `json:"step_id"`
	Result ToolResult `json:"result"`
}

type TrajectoryEvaluationRequest struct {
	TaskID string `json:"task_id"`
}
