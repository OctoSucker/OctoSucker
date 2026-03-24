package utils

import (
	"maps"

	"github.com/OctoSucker/agent/pkg/ports"
)

// PlanStepArguments returns a clone of arguments for the step id, or nil.
func PlanStepArguments(sess *ports.Session, stepID string) map[string]any {
	if sess == nil || sess.Plan == nil {
		return nil
	}
	for i := range sess.Plan.Steps {
		if sess.Plan.Steps[i].ID == stepID {
			return maps.Clone(sess.Plan.Steps[i].Arguments)
		}
	}
	return nil
}

// ToolFailCountKey is a stable key for per-step tool failure counts in a map.
func ToolFailCountKey(stepID, tool string) string { return stepID + "\x1e" + tool }

// CapabilityFailCountKey is a stable key for per-step capability failure counts in a map.
func CapabilityFailCountKey(stepID, capability string) string { return stepID + "\x1e" + capability }
