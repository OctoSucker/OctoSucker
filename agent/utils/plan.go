package utils

import "github.com/OctoSucker/agent/pkg/ports"

// FindPlanStep returns a pointer to the step with the given id, or nil.
func FindPlanStep(p *ports.Plan, stepID string) *ports.PlanStep {
	if p == nil {
		return nil
	}
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			return &p.Steps[i]
		}
	}
	return nil
}

// RouteSearchGroups orders candidate capability lists by routing mode.
func RouteSearchGroups(routeMode ports.RouteMode, frontier, skillPath, skillPrior []string) [][]string {
	if routeMode == ports.RouteGraph {
		return [][]string{frontier, skillPath, skillPrior}
	}
	return [][]string{skillPath, skillPrior, frontier}
}
