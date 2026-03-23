package ports

type PayloadUserInput struct {
	SessionID  string `json:"session_id"`
	Text       string `json:"text"`
	AutoReplan bool   `json:"auto_replan,omitempty"`
}

type PayloadPlanCreated struct {
	SessionID string `json:"session_id"`
}

type PayloadToolsBound struct {
	SessionID  string   `json:"session_id"`
	StepID     string   `json:"step_id"`
	Capability string   `json:"capability"`
	Tools      []string `json:"tools"`
}

type PayloadToolCall struct {
	SessionID  string         `json:"session_id"`
	StepID     string         `json:"step_id"`
	Capability string         `json:"capability"`
	Tool       string         `json:"tool"`
	Arguments  map[string]any `json:"arguments,omitempty"`
}

type PayloadObservation struct {
	SessionID  string      `json:"session_id"`
	StepID     string      `json:"step_id"`
	Capability string      `json:"capability"`
	Tool       string      `json:"tool"`
	Obs        interface{} `json:"obs"`
}

type PayloadStepCompleted struct {
	SessionID string `json:"session_id"`
}

type PayloadStepCapabilityRetry struct {
	SessionID         string `json:"session_id"`
	StepID            string `json:"step_id"`
	ExcludeCapability string `json:"exclude_capability"`
}

type PayloadTrajectoryCheck struct {
	SessionID string `json:"session_id"`
}

type PayloadTurnFinalized struct {
	SessionID string `json:"session_id"`
}
