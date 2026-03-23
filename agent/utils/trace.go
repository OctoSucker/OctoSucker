package utils

import "github.com/OctoSucker/agent/pkg/ports"

// TraceHasFailure is true if any step trace is not OK.
func TraceHasFailure(tr []ports.StepTrace) bool {
	for i := range tr {
		if !tr[i].OK {
			return true
		}
	}
	return false
}
