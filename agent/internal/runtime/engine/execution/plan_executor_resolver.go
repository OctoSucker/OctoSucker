package execution

import (
	"context"
	"fmt"

	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
)

func (x *PlanExecutor) capabilityAllowed(allow map[string]bool, id, exclude string) bool {
	if id == "" {
		return false
	}
	if exclude != "" && id == exclude {
		return false
	}
	return allow[id] && x.CapRegistry.FirstTool(id) != ""
}

func addCandidate(candidates []string, seen map[string]struct{}, id string, allowed bool) []string {
	if !allowed {
		return candidates
	}
	if _, dup := seen[id]; dup {
		return candidates
	}
	seen[id] = struct{}{}
	return append(candidates, id)
}

func (x *PlanExecutor) resolvePlanCapability(ctx context.Context, snap ports.RouteSnap, want, exclude string) (string, error) {
	rc := ports.RoutingContext{IntentText: snap.UserInput}
	frontier, err := x.RouteGraph.Frontier(ctx, rc, snap.LastCap, snap.LastOut)
	if err != nil {
		return "", fmt.Errorf("plan_executor: Frontier: %w", err)
	}
	allow := buildAllowedCapabilities(frontier, snap.ProcedurePrior, snap.Preferred)
	if x.capabilityAllowed(allow, want, exclude) {
		return want, nil
	}
	seen := map[string]struct{}{}
	var candidates []string
	candidates = addCandidate(candidates, seen, want, x.capabilityAllowed(allow, want, exclude))
	for _, g := range rtutils.RouteSearchGroups(snap.RouteType, frontier, snap.Preferred, snap.ProcedurePrior) {
		for _, c := range g {
			candidates = addCandidate(candidates, seen, c, x.capabilityAllowed(allow, c, exclude))
		}
	}
	for _, c := range snap.ProcedurePrior {
		candidates = addCandidate(candidates, seen, c, x.capabilityAllowed(allow, c, exclude))
	}
	if snap.GraphPathMode == ports.GraphPathGlobal {
		if picked, ok := x.RouteGraph.PickBestByImmediateEdge(ctx, rc, snap.LastCap, candidates); ok {
			return picked, nil
		}
	}
	for _, g := range rtutils.RouteSearchGroups(snap.RouteType, frontier, snap.Preferred, snap.ProcedurePrior) {
		for _, c := range g {
			if x.capabilityAllowed(allow, c, exclude) {
				return c, nil
			}
		}
	}
	for _, c := range snap.ProcedurePrior {
		if x.capabilityAllowed(allow, c, exclude) {
			return c, nil
		}
	}
	if exclude != "" {
		return "", nil
	}
	return want, nil
}

func buildAllowedCapabilities(frontier, procedurePrior, preferred []string) map[string]bool {
	allow := make(map[string]bool, len(frontier)+len(procedurePrior)+len(preferred))
	for _, capID := range frontier {
		allow[capID] = true
	}
	for _, capID := range procedurePrior {
		allow[capID] = true
	}
	for _, capID := range preferred {
		allow[capID] = true
	}
	return allow
}
