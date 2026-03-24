package execution

import (
	"context"
	"fmt"
	"maps"
	"sort"

	"github.com/OctoSucker/agent/internal/runtime/store/capability"
	routinggraph "github.com/OctoSucker/agent/internal/runtime/store/routing_graph"
	"github.com/OctoSucker/agent/internal/runtime/store/session"
	skill "github.com/OctoSucker/agent/internal/runtime/store/skill"
	"github.com/OctoSucker/agent/pkg/mcpclient"
	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
	"golang.org/x/sync/errgroup"
)

type PlanExecutor struct {
	Sessions    *session.SessionStore
	RouteGraph  *routinggraph.RoutingGraph
	CapRegistry *capability.CapabilityRegistry
	MCPRouter   *mcpclient.MCPRouter
}

type stepWaveResult struct {
	StepID   string
	Trace    []ports.StepTrace
	FinalCap string
	Out      int
}

// HandlePlanCreated continues execution after a new plan is attached to the session.
func (x *PlanExecutor) HandlePlanCreated(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	pl := evt.Payload.(ports.PayloadPlanCreated)
	return x.handleAfterRunnableProgress(ctx, pl.SessionID)
}

// HandleStepCompleted continues execution after a step finishes (single-tool path or wave handoff).
func (x *PlanExecutor) HandleStepCompleted(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	pl := evt.Payload.(ports.PayloadStepCompleted)
	return x.handleAfterRunnableProgress(ctx, pl.SessionID)
}

// HandleStepCapabilityRetry switches the step to another capability and restarts that step.
func (x *PlanExecutor) HandleStepCapabilityRetry(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	pl := evt.Payload.(ports.PayloadStepCapabilityRetry)
	sess, ok := x.Sessions.Get(pl.SessionID)
	if !ok || sess.Plan == nil {
		return nil, nil
	}
	st := rtutils.FindPlanStep(sess.Plan, pl.StepID)
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
}

// handleAfterRunnableProgress runs the next runnable step(s) or emits trajectory check when the plan is done.
func (x *PlanExecutor) handleAfterRunnableProgress(ctx context.Context, sid string) ([]ports.Event, error) {
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
}

func (x *PlanExecutor) routingGraphGoal() string {
	if x.CapRegistry != nil && len(x.CapRegistry.Tools("finish")) > 0 {
		return "finish"
	}
	return ""
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
	for _, g := range rtutils.RouteSearchGroups(snap.RouteMode, frontier, snap.Preferred, snap.SkillPrior) {
		for _, c := range g {
			addCand(c)
		}
	}
	for _, c := range snap.SkillPrior {
		addCand(c)
	}
	if snap.GraphPathMode == ports.GraphPathGlobal {
		if goal := x.routingGraphGoal(); goal != "" {
			if picked, ok := x.RouteGraph.PickGlobalBestNext(ctx, rc, snap.LastCap, goal, candidates); ok {
				return picked
			}
		}
	}
	for _, g := range rtutils.RouteSearchGroups(snap.RouteMode, frontier, snap.Preferred, snap.SkillPrior) {
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
	st := rtutils.FindPlanStep(sess.Plan, stepID)
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
	argMap := skill.RenderPlanStepArguments(sess, st.ID)
	if argMap == nil {
		argMap = maps.Clone(st.Arguments)
	}
	return []ports.Event{{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{
		SessionID: sess.ID, StepID: st.ID, Capability: st.Capability, Tool: tools[0], Arguments: argMap,
	}}}, nil
}

func (x *PlanExecutor) runPlanStepWave(ctx context.Context, sess *ports.Session, st ports.PlanStep) (stepWaveResult, error) {
	capID := x.resolvePlanCapability(ctx, sess.RouteSnap(), st.Capability, "")
	if capID != st.Capability {
		st.Capability = capID
	}
	tools := x.CapRegistry.Tools(st.Capability)
	if len(tools) == 0 {
		return stepWaveResult{}, fmt.Errorf("plan_executor wave: capability %q has no tools", st.Capability)
	}
	argMap := skill.RenderPlanStepArguments(sess, st.ID)
	if argMap == nil {
		argMap = maps.Clone(st.Arguments)
	}
	trace := []ports.StepTrace{}
	outcome := 0
	for _, tool := range tools {
		res, err := x.MCPRouter.Invoke(ctx, ports.CapabilityInvocation{CapabilityID: st.Capability, Tool: tool, Arguments: maps.Clone(argMap)})
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
	for i := range run {
		i, st := i, run[i]
		g.Go(func() error {
			w, err := x.runPlanStepWave(gctx, sess, st)
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
