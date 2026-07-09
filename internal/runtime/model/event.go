// Package model holds task/plan state, tool I/O, and dispatcher event/payload types.
package model

// Event is a dispatcher state-transition request.
type Event struct {
	Type    string
	Payload any
}

func EventPtr(e Event) *Event { return &e }

// TaskIDFromEvent returns the task id in evt's payload for known event types.
func TaskIDFromEvent(evt Event) (string, bool) {
	switch evt.Type {
	case EvTurnRequested:
		p, ok := evt.Payload.(TurnRequest)
		if !ok {
			return "", false
		}
		return p.TaskID, p.TaskID != ""
	case EvStepScheduled:
		p, ok := evt.Payload.(StepScheduled)
		if !ok {
			return "", false
		}
		return p.TaskID, p.TaskID != ""
	case EvStepObserved:
		p, ok := evt.Payload.(StepObserved)
		if !ok {
			return "", false
		}
		return p.TaskID, p.TaskID != ""
	case EvTrajectoryEvaluationRequested:
		p, ok := evt.Payload.(TrajectoryEvaluationRequest)
		if !ok {
			return "", false
		}
		return p.TaskID, p.TaskID != ""
	default:
		return "", false
	}
}

const (
	EvTurnRequested                 = "TurnRequested"
	EvStepScheduled                 = "StepScheduled"
	EvStepObserved                  = "StepObserved"
	EvTrajectoryEvaluationRequested = "TrajectoryEvaluationRequested"
)
