package judge

import (
	"context"
	"fmt"

	"github.com/OctoSucker/agent/pkg/ports"
)

func (x *StepCritic) trySwitchCapability(ctx context.Context, sess *ports.Task, exclude string) (string, error) {
	if x.RouteGraph == nil || x.CapRegistry == nil {
		return "", nil
	}
	rc := ports.RoutingContext{IntentText: sess.UserInput.Text}
	frontier, err := x.RouteGraph.Frontier(ctx, rc, sess.LastCapability, 1)
	if err != nil {
		return "", fmt.Errorf("step_critic: Frontier: %w", err)
	}
	if len(frontier) == 0 {
		frontier, err = x.RouteGraph.EntryNodes(ctx, rc)
		if err != nil {
			return "", fmt.Errorf("step_critic: EntryNodes: %w", err)
		}
	}
	allow := map[string]bool{}
	for _, capID := range frontier {
		if capID != exclude {
			allow[capID] = true
		}
	}
	for _, capID := range sess.SkillPriorCaps {
		if capID != exclude {
			allow[capID] = true
		}
	}
	for capID := range allow {
		if x.CapRegistry.FirstTool(capID) != "" {
			return capID, nil
		}
	}
	return "", nil
}
