package utils

import (
	"maps"

	"github.com/OctoSucker/agent/pkg/ports"
)

// PlanStepArguments returns a clone of arguments for the step id, or nil.
func PlanStepArguments(taskState *ports.Task, stepID string) map[string]any {
	if taskState == nil || taskState.Plan == nil {
		return nil
	}
	for i := range taskState.Plan.Steps {
		if taskState.Plan.Steps[i].ID == stepID {
			return maps.Clone(taskState.Plan.Steps[i].Arguments)
		}
	}
	return nil
}

// ToolFailCountKey is a stable key for per-step tool failure counts in a map.
func ToolFailCountKey(stepID, tool string) string { return stepID + "\x1e" + tool }

// CapabilityFailCountKey is a stable key for per-step capability failure counts in a map.
func CapabilityFailCountKey(stepID, capability string) string { return stepID + "\x1e" + capability }
