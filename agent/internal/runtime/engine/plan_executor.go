package engine

import (
	"context"
	"fmt"
	"maps"
	"sort"

	"github.com/OctoSucker/agent/pkg/mcpclient"
	"github.com/OctoSucker/agent/pkg/ports"
	"golang.org/x/sync/errgroup"
)

func (d *Dispatcher) planExecutorPlanProgress(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	switch evt.Type {
	case ports.EvPlanCreated, ports.EvStepCompleted:
		var sid string
		switch pl := evt.Payload.(type) {
		case ports.PayloadPlanCreated:
			sid = pl.SessionID
		case ports.PayloadStepCompleted:
			sid = pl.SessionID
		}
		sess, ok := d.Sessions.Get(sid)
		if !ok || sess.Plan == nil {
			return nil, nil
		}
		run := sess.Plan.Runnable()
		if len(run) == 0 {
			if sess.Plan.AllDone() {
				return []ports.Event{{Type: ports.EvTrajectoryCheck, Payload: ports.PayloadTrajectoryCheck{SessionID: sid}}}, nil
			}
			return nil, nil
		}
		if len(run) > 1 {
			return d.runParallelStepWave(ctx, sid, sess, run)
		}
		return d.startPlanStep(ctx, sess, run[0].ID)
	case ports.EvStepCapabilityRetry:
		pl := evt.Payload.(ports.PayloadStepCapabilityRetry)
		sess, ok := d.Sessions.Get(pl.SessionID)
		if !ok || sess.Plan == nil {
			return nil, nil
		}
		st := findPlanStep(sess.Plan, pl.StepID)
		if st == nil {
			return nil, nil
		}
		snap := sess.RouteSnap()
		capID := d.resolvePlanCapability(ctx, snap, st.Capability, pl.ExcludeCapability)
		if capID == "" {
			return nil, fmt.Errorf("plan_executor: no alternative capability for step %q", pl.StepID)
		}
		st.Capability = capID
		sess.CapChainStepID = ""
		sess.CapChainTools = nil
		sess.CapChainNext = 0
		sess.StepID = ""
		sess.PendingTool = ""
		return d.startPlanStep(ctx, sess, pl.StepID)
	default:
		return nil, nil
	}
}

type stepWaveResult struct {
	StepID   string
	Trace    []ports.StepTrace
	FinalCap string
	Out      int
}

func routingGroups(routeMode ports.RouteMode, frontier, skillPath, skillPrior []string) [][]string {
	switch routeMode {
	case ports.RouteGraph:
		return [][]string{frontier, skillPath, skillPrior}
	default:
		return [][]string{skillPath, skillPrior, frontier}
	}
}

func findPlanStep(p *ports.Plan, stepID string) *ports.PlanStep {
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

func (d *Dispatcher) resolvePlanCapability(ctx context.Context, snap ports.RouteSnap, want, exclude string) string {
	if d.RouteGraph == nil || d.CapRegistry == nil {
		if exclude != "" {
			if want != exclude {
				return want
			}
			return ""
		}
		return want
	}
	rc := ports.RoutingContext{IntentText: snap.UserInput}
	frontier, _ := d.RouteGraph.Frontier(ctx, rc, snap.LastCap, snap.LastOut)
	if len(frontier) == 0 {
		frontier, _ = d.RouteGraph.EntryNodes(ctx, rc)
	}
	allow := make(map[string]bool)
	for _, c := range frontier {
		allow[c] = true
	}
	for _, c := range snap.SkillPrior {
		allow[c] = true
	}
	for _, c := range snap.Preferred {
		allow[c] = true
	}
	ok := func(id string) bool {
		return allow[id] && d.CapRegistry.FirstTool(id) != ""
	}
	filter := ok
	if exclude != "" {
		filter = func(id string) bool { return id != exclude && ok(id) }
	}
	if filter(want) {
		return want
	}
	for _, g := range routingGroups(snap.RouteMode, frontier, snap.Preferred, snap.SkillPrior) {
		for _, c := range g {
			if filter(c) {
				return c
			}
		}
	}
	for _, c := range snap.SkillPrior {
		if filter(c) {
			return c
		}
	}
	if exclude != "" {
		return ""
	}
	return want
}

func (d *Dispatcher) startPlanStep(ctx context.Context, sess *ports.Session, stepID string) ([]ports.Event, error) {
	st := findPlanStep(sess.Plan, stepID)
	if st == nil {
		return nil, nil
	}
	snap := sess.RouteSnap()
	capID := d.resolvePlanCapability(ctx, snap, st.Capability, "")
	if capID != st.Capability {
		st.Capability = capID
	}
	tools := d.CapRegistry.Tools(st.Capability)
	if len(tools) == 0 {
		return nil, fmt.Errorf("plan_executor: capability %q has no tools", st.Capability)
	}
	sess.Plan.MarkRunning(st.ID)
	if len(tools) == 1 {
		sess.CapChainStepID = ""
		sess.CapChainTools = nil
		sess.CapChainNext = 0
	} else {
		sess.CapChainStepID = st.ID
		sess.CapChainTools = tools
		sess.CapChainNext = 0
	}
	sess.StepID = st.ID
	sess.PendingTool = tools[0]
	if err := d.Sessions.Put(sess); err != nil {
		return nil, err
	}
	return []ports.Event{{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{
		SessionID: sess.ID, StepID: st.ID, Capability: st.Capability, Tool: tools[0],
		Arguments: maps.Clone(st.Arguments),
	}}}, nil
}

func (d *Dispatcher) runPlanStepWave(ctx context.Context, snap ports.RouteSnap, st ports.PlanStep, router *mcpclient.MCPRouter) (stepWaveResult, error) {
	capID := d.resolvePlanCapability(ctx, snap, st.Capability, "")
	if capID != st.Capability {
		st.Capability = capID
	}
	tools := d.CapRegistry.Tools(st.Capability)
	if len(tools) == 0 {
		return stepWaveResult{}, fmt.Errorf("plan_executor wave: capability %q has no tools", st.Capability)
	}
	var trace []ports.StepTrace
	outcome := 0
	finalCap := st.Capability
	args := maps.Clone(st.Arguments)
	for _, tool := range tools {
		res, err := router.Invoke(ctx, ports.CapabilityInvocation{
			CapabilityID: st.Capability,
			Tool:         tool,
			Arguments:    args,
		})
		if err != nil {
			res = ports.ToolResult{OK: false, Err: err}
		}
		obs := res.Observation()
		trace = append(trace, ports.StepTrace{StepID: st.ID, Tool: tool, OK: obs.Err == nil, Summary: obs.Summary})
		if obs.Err != nil {
			outcome = 1
			break
		}
	}
	return stepWaveResult{StepID: st.ID, Trace: trace, FinalCap: finalCap, Out: outcome}, nil
}

func (d *Dispatcher) runParallelStepWave(ctx context.Context, sid string, sess *ports.Session, run []ports.PlanStep) ([]ports.Event, error) {
	snap := sess.RouteSnap()
	for i := range run {
		sess.Plan.MarkRunning(run[i].ID)
	}
	sess.CapChainStepID = ""
	sess.CapChainTools = nil
	sess.CapChainNext = 0
	sess.StepID = ""
	sess.PendingTool = ""
	results := make([]stepWaveResult, len(run))
	g, gctx := errgroup.WithContext(ctx)
	for i := range run {
		i, st := i, run[i]
		g.Go(func() error {
			wave, err := d.runPlanStepWave(gctx, snap, st, d.MCPRouter)
			if err != nil {
				return err
			}
			results[i] = wave
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	order := make(map[string]int)
	for i := range sess.Plan.Steps {
		order[sess.Plan.Steps[i].ID] = i
	}
	sort.Slice(results, func(a, b int) bool {
		return order[results[a].StepID] < order[results[b].StepID]
	})
	lastCap := sess.LastCapability
	for i := range results {
		res := results[i]
		sess.Trace = append(sess.Trace, res.Trace...)
		if d.RouteGraph != nil {
			_ = d.RouteGraph.RecordTransition(ctx, ports.RoutingContext{IntentText: sess.UserInput}, lastCap, res.FinalCap, res.Out)
			sess.TransitionPath = append(sess.TransitionPath, ports.TransitionStep{From: lastCap, To: res.FinalCap})
		}
		lastCap = res.FinalCap
		sess.Plan.MarkDone(res.StepID)
	}
	sess.LastCapability = lastCap
	if len(results) > 0 {
		sess.LastOutcome = results[len(results)-1].Out
	}
	if err := d.Sessions.Put(sess); err != nil {
		return nil, err
	}
	if sess.Plan.AllDone() {
		return []ports.Event{{Type: ports.EvTrajectoryCheck, Payload: ports.PayloadTrajectoryCheck{SessionID: sid}}}, nil
	}
	return []ports.Event{{Type: ports.EvStepCompleted, Payload: ports.PayloadStepCompleted{SessionID: sid}}}, nil
}
