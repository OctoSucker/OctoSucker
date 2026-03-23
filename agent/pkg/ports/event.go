package ports

type Event struct {
	Type    string
	Payload any
}

const (
	EvUserInput           = "UserInput"
	EvPlanCreated         = "PlanCreated"
	EvToolsBound          = "ToolsBound"
	EvToolCall            = "ToolCall"
	EvObservationReady    = "ObservationReady"
	EvStepCompleted       = "StepCompleted"
	EvStepCapabilityRetry = "StepCapabilityRetry"
	EvTrajectoryCheck     = "TrajectoryCheck"
	EvTurnFinalized       = "TurnFinalized"
)
