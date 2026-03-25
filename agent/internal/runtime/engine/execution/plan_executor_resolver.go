package execution

import (
	"context"
	"fmt"

	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
)

func (x *PlanExecutor) resolvePlanCapability(ctx context.Context, snap ports.RouteSnap, want, exclude string) (string, error) {
	if x.RouteGraph == nil || x.CapRegistry == nil {
		if exclude != "" && want == exclude {
			return "", nil
		}
		return want, nil
	}
	rc := ports.RoutingContext{IntentText: snap.UserInput}
	frontier, err := x.RouteGraph.Frontier(ctx, rc, snap.LastCap, snap.LastOut)
	if err != nil {
		return "", fmt.Errorf("plan_executor: Frontier: %w", err)
	}
	if len(frontier) == 0 {
		frontier, err = x.RouteGraph.EntryNodes(ctx, rc)
		if err != nil {
			return "", fmt.Errorf("plan_executor: EntryNodes: %w", err)
		}
	}
	allow := map[string]bool{}
	for _, c := range append(append(frontier, snap.SkillPrior...), snap.Preferred...) {
		allow[c] = true
	}
	ok := func(id string) bool { return allow[id] && x.CapRegistry.FirstTool(id) != "" }
	filter := ok
	if exclude != "" {
		filter = func(id string) bool { return id != exclude && ok(id) }
	}
	if filter(want) {
		return want, nil
	}
	seen := map[string]struct{}{}
	var candidates []string
	addCand := func(id string) {
		if id == "" || !filter(id) {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
		candidates = append(candidates, id)
	}
	addCand(want)
	for _, g := range rtutils.RouteSearchGroups(snap.RouteType, frontier, snap.Preferred, snap.SkillPrior) {
		for _, c := range g {
			addCand(c)
		}
	}
	for _, c := range snap.SkillPrior {
		addCand(c)
	}
	if snap.GraphPathMode == ports.GraphPathGlobal {
		if picked, ok := x.RouteGraph.PickBestByImmediateEdge(ctx, rc, snap.LastCap, candidates); ok {
			return picked, nil
		}
	}
	for _, g := range rtutils.RouteSearchGroups(snap.RouteType, frontier, snap.Preferred, snap.SkillPrior) {
		for _, c := range g {
			if filter(c) {
				return c, nil
			}
		}
	}
	for _, c := range snap.SkillPrior {
		if filter(c) {
			return c, nil
		}
	}
	if exclude != "" {
		return "", nil
	}
	return want, nil
}
