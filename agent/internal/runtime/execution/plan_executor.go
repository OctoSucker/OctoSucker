package execution

import (
	"context"
	"fmt"
	"maps"
	"sort"

	"github.com/OctoSucker/agent/pkg/ports"
	"golang.org/x/sync/errgroup"
)

type CapabilityTools interface {
	FirstTool(capID string) string
	Tools(capID string) []string
}

type PlanExecutor struct {
	Sessions    FullSessionRepository
	RouteGraph  RouteGraph
	CapRegistry CapabilityTools
	MCPRouter   ToolInvoker
}

type stepWaveResult struct {
	StepID   string
	Trace    []ports.StepTrace
	FinalCap string
	Out      int
}

func (x *PlanExecutor) HandlePlanProgress(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	switch evt.Type {
	case ports.EvPlanCreated, ports.EvStepCompleted:
		var sid string
		switch pl := evt.Payload.(type) {
		case ports.PayloadPlanCreated:
			sid = pl.SessionID
		case ports.PayloadStepCompleted:
			sid = pl.SessionID
		}
		sess, ok := x.Sessions.Get(sid)
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
			return x.runParallelStepWave(ctx, sid, sess, run)
		}
		return x.startPlanStep(ctx, sess, run[0].ID)
	case ports.EvStepCapabilityRetry:
		pl := evt.Payload.(ports.PayloadStepCapabilityRetry)
		sess, ok := x.Sessions.Get(pl.SessionID)
		if !ok || sess.Plan == nil {
			return nil, nil
		}
		st := findPlanStep(sess.Plan, pl.StepID)
		if st == nil {
			return nil, nil
		}
		snap := sess.RouteSnap()
		capID := x.resolvePlanCapability(ctx, snap, st.Capability, pl.ExcludeCapability)
		if capID == "" {
			return nil, fmt.Errorf("plan_executor: no alternative capability for step %q", pl.StepID)
		}
		st.Capability = capID
		sess.CapChainStepID, sess.CapChainTools, sess.CapChainNext, sess.StepID, sess.PendingTool = "", nil, 0, "", ""
		return x.startPlanStep(ctx, sess, pl.StepID)
	default:
		return nil, nil
	}
}

func routingGroups(routeMode ports.RouteMode, frontier, skillPath, skillPrior []string) [][]string {
	if routeMode == ports.RouteGraph {
		return [][]string{frontier, skillPath, skillPrior}
	}
	return [][]string{skillPath, skillPrior, frontier}
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

func (x *PlanExecutor) resolvePlanCapability(ctx context.Context, snap ports.RouteSnap, want, exclude string) string {
	if x.RouteGraph == nil || x.CapRegistry == nil {
		if exclude != "" && want == exclude {
			return ""
		}
		return want
	}
	rc := ports.RoutingContext{IntentText: snap.UserInput}
	frontier, _ := x.RouteGraph.Frontier(ctx, rc, snap.LastCap, snap.LastOut)
	if len(frontier) == 0 {
		frontier, _ = x.RouteGraph.EntryNodes(ctx, rc)
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

func (x *PlanExecutor) startPlanStep(ctx context.Context, sess *ports.Session, stepID string) ([]ports.Event, error) {
	st := findPlanStep(sess.Plan, stepID)
	if st == nil {
		return nil, nil
	}
	capID := x.resolvePlanCapability(ctx, sess.RouteSnap(), st.Capability, "")
	if capID != st.Capability {
		st.Capability = capID
	}
	tools := x.CapRegistry.Tools(st.Capability)
	if len(tools) == 0 {
		return nil, fmt.Errorf("plan_executor: capability %q has no tools", st.Capability)
	}
	sess.Plan.MarkRunning(st.ID)
	if len(tools) == 1 {
		sess.CapChainStepID, sess.CapChainTools, sess.CapChainNext = "", nil, 0
	} else {
		sess.CapChainStepID, sess.CapChainTools, sess.CapChainNext = st.ID, tools, 0
	}
	sess.StepID, sess.PendingTool = st.ID, tools[0]
	if err := x.Sessions.Put(sess); err != nil {
		return nil, err
	}
	return []ports.Event{{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{
		SessionID: sess.ID, StepID: st.ID, Capability: st.Capability, Tool: tools[0], Arguments: maps.Clone(st.Arguments),
	}}}, nil
}

func (x *PlanExecutor) runPlanStepWave(ctx context.Context, snap ports.RouteSnap, st ports.PlanStep) (stepWaveResult, error) {
	capID := x.resolvePlanCapability(ctx, snap, st.Capability, "")
	if capID != st.Capability {
		st.Capability = capID
	}
	tools := x.CapRegistry.Tools(st.Capability)
	if len(tools) == 0 {
		return stepWaveResult{}, fmt.Errorf("plan_executor wave: capability %q has no tools", st.Capability)
	}
	trace := []ports.StepTrace{}
	outcome := 0
	for _, tool := range tools {
		res, err := x.MCPRouter.Invoke(ctx, ports.CapabilityInvocation{CapabilityID: st.Capability, Tool: tool, Arguments: maps.Clone(st.Arguments)})
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
	return stepWaveResult{StepID: st.ID, Trace: trace, FinalCap: st.Capability, Out: outcome}, nil
}

func (x *PlanExecutor) runParallelStepWave(ctx context.Context, sid string, sess *ports.Session, run []ports.PlanStep) ([]ports.Event, error) {
	for i := range run {
		sess.Plan.MarkRunning(run[i].ID)
	}
	sess.CapChainStepID, sess.CapChainTools, sess.CapChainNext, sess.StepID, sess.PendingTool = "", nil, 0, "", ""
	results := make([]stepWaveResult, len(run))
	g, gctx := errgroup.WithContext(ctx)
	snap := sess.RouteSnap()
	for i := range run {
		i, st := i, run[i]
		g.Go(func() error {
			w, err := x.runPlanStepWave(gctx, snap, st)
			if err == nil {
				results[i] = w
			}
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	order := map[string]int{}
	for i := range sess.Plan.Steps {
		order[sess.Plan.Steps[i].ID] = i
	}
	sort.Slice(results, func(a, b int) bool { return order[results[a].StepID] < order[results[b].StepID] })
	lastCap := sess.LastCapability
	for _, res := range results {
		sess.Trace = append(sess.Trace, res.Trace...)
		if x.RouteGraph != nil {
			_ = x.RouteGraph.RecordTransition(ctx, ports.RoutingContext{IntentText: sess.UserInput}, lastCap, res.FinalCap, res.Out)
			sess.TransitionPath = append(sess.TransitionPath, ports.TransitionStep{From: lastCap, To: res.FinalCap})
		}
		lastCap = res.FinalCap
		sess.Plan.MarkDone(res.StepID)
	}
	sess.LastCapability = lastCap
	if len(results) > 0 {
		sess.LastOutcome = results[len(results)-1].Out
	}
	if err := x.Sessions.Put(sess); err != nil {
		return nil, err
	}
	if sess.Plan.AllDone() {
		return []ports.Event{{Type: ports.EvTrajectoryCheck, Payload: ports.PayloadTrajectoryCheck{SessionID: sid}}}, nil
	}
	return []ports.Event{{Type: ports.EvStepCompleted, Payload: ports.PayloadStepCompleted{SessionID: sid}}}, nil
}
