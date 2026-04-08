package types

type PayloadUserInput struct {
	TaskID string `json:"task_id"`
	Text   string `json:"text"`
}

type PayloadPlanProgressed struct {
	TaskID string `json:"task_id"`
}

type PayloadObservation struct {
	TaskID string     `json:"task_id"`
	StepID string     `json:"step_id"`
	Result ToolResult `json:"result"`
}

type PayloadTrajectoryCheck struct {
	TaskID string `json:"task_id"`
}
