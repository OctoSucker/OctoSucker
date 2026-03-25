package ports

type PayloadUserInput struct {
	TaskID     string `json:"task_id"`
	Text       string `json:"text"`
	AutoReplan bool   `json:"auto_replan,omitempty"`
	// TelegramChatID is set when the message came from Telegram but TaskID is a shared conversation key (see workspace conversation_id).
	TelegramChatID int64 `json:"telegram_chat_id,omitempty"`
}

type PayloadPlanProgressed struct {
	TaskID string `json:"task_id"`
}

type PayloadSkillPlanRequested struct {
	TaskID string `json:"task_id"`
}

type PayloadLLMPlanRequested struct {
	TaskID string `json:"task_id"`
}

type PayloadToolsBound struct {
	TaskID     string   `json:"task_id"`
	StepID     string   `json:"step_id"`
	Capability string   `json:"capability"`
	Tools      []string `json:"tools"`
}

type PayloadToolCall struct {
	TaskID     string         `json:"task_id"`
	StepID     string         `json:"step_id"`
	Capability string         `json:"capability"`
	Tool       string         `json:"tool"`
	Arguments  map[string]any `json:"arguments,omitempty"`
}

type PayloadObservation struct {
	TaskID     string      `json:"task_id"`
	StepID     string      `json:"step_id"`
	Capability string      `json:"capability"`
	Tool       string      `json:"tool"`
	Obs        Observation `json:"obs"`
}

type PayloadStepCapabilityRetry struct {
	TaskID            string `json:"task_id"`
	StepID            string `json:"step_id"`
	ExcludeCapability string `json:"exclude_capability"`
}

type PayloadTrajectoryCheck struct {
	TaskID string `json:"task_id"`
}

type PayloadTurnFinalized struct {
	TaskID string `json:"task_id"`
}
