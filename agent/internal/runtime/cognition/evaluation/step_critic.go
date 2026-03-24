package evaluation

import (
	"context"

	"github.com/OctoSucker/agent/internal/runtime/store/capability"
	"github.com/OctoSucker/agent/internal/runtime/store/nodefailure"
	routinggraph "github.com/OctoSucker/agent/internal/runtime/store/routing_graph"
	"github.com/OctoSucker/agent/internal/runtime/store/session"
	skill "github.com/OctoSucker/agent/internal/runtime/store/skill"
	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
)

type StepCritic struct {
	Sessions              *session.SessionStore
	RouteGraph            *routinggraph.RoutingGraph
	CapRegistry           *capability.CapabilityRegistry
	NodeFailures          *nodefailure.NodeFailureStats
	MaxFailsPerTool       int
	MaxFailsPerCapability int // per stepID+capability failure count before preferring switch (0 = use default 2)
}

func NewStepCritic(sessions *session.SessionStore, routeGraph *routinggraph.RoutingGraph, capRegistry *capability.CapabilityRegistry, nodeFailures *nodefailure.NodeFailureStats, maxFailsPerTool int, maxFailsPerCapability int) *StepCritic {
	return &StepCritic{Sessions: sessions, RouteGraph: routeGraph, CapRegistry: capRegistry, NodeFailures: nodeFailures, MaxFailsPerTool: maxFailsPerTool, MaxFailsPerCapability: maxFailsPerCapability}
}

func (x *StepCritic) maxStepToolFails() int {
	if x.MaxFailsPerTool <= 0 {
		panic("evaluation.StepCritic: MaxFailsPerTool must be > 0")
	}
	return x.MaxFailsPerTool
}

func (x *StepCritic) maxCapabilityFails() int {
	if x.MaxFailsPerCapability < 1 {
		return 2
	}
	return x.MaxFailsPerCapability
}

func (x *StepCritic) bumpCapabilityFailCount(sess *ports.Session, stepID, capability string) int {
	if sess.CapabilityFailCount == nil {
		sess.CapabilityFailCount = make(map[string]int)
	}
	k := rtutils.CapabilityFailCountKey(stepID, capability)
	sess.CapabilityFailCount[k]++
	return sess.CapabilityFailCount[k]
}

func (x *StepCritic) HandleObservationReady(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	pl := evt.Payload.(ports.PayloadObservation)
	obs := pl.Obs.(ports.Observation)
	sess, ok := x.Sessions.Get(pl.SessionID)
	if !ok {
		return nil, nil
	}
	sess.StepID = pl.StepID
	sess.Trace = append(sess.Trace, ports.StepTrace{StepID: pl.StepID, Tool: pl.Tool, OK: obs.Err == nil, Summary: obs.Summary})
	if obs.Err != nil && x.NodeFailures != nil {
		msg := obs.Err.Error()
		if msg == "" {
			msg = obs.Summary
		}
		_ = x.NodeFailures.RecordFailure(pl.Capability, pl.Tool, sess.LastCapability, msg)
	}
	if obs.Err == nil && sess.ToolFailCount != nil {
		delete(sess.ToolFailCount, rtutils.ToolFailCountKey(pl.StepID, pl.Tool))
	}
	if obs.Err == nil && sess.CapabilityFailCount != nil {
		delete(sess.CapabilityFailCount, rtutils.CapabilityFailCountKey(pl.StepID, pl.Capability))
	}
	if pl.StepID == sess.CapChainStepID && len(sess.CapChainTools) > 1 {
		if obs.Err != nil {
			return x.handleCapChainToolFailure(ctx, sess, pl)
		}
		sess.CapChainNext++
		if sess.CapChainNext < len(sess.CapChainTools) {
			sess.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "next in chain"}
			sess.PendingTool = sess.CapChainTools[sess.CapChainNext]
			if err := x.Sessions.Put(sess); err != nil {
				return nil, err
			}
			return []ports.Event{{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{SessionID: pl.SessionID, StepID: pl.StepID, Capability: pl.Capability, Tool: sess.PendingTool, Arguments: planStepArgsForTool(sess, pl.StepID)}}}, nil
		}
		sess.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "chain done"}
		sess.CapChainStepID, sess.CapChainTools, sess.CapChainNext = "", nil, 0
	} else if obs.Err != nil && len(sess.CapChainTools) <= 1 {
		ev, err := x.handleSingleToolFailure(ctx, sess, pl)
		if err != nil || ev != nil {
			return ev, err
		}
	}
	outcome := 0
	if obs.Err != nil {
		outcome = 1
	}
	if sess.LastStepDecision == nil {
		sess.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "step done"}
	}
	if x.RouteGraph != nil {
		_ = x.RouteGraph.RecordTransition(ctx, ports.RoutingContext{IntentText: sess.UserInput}, sess.LastCapability, pl.Capability, outcome)
		sess.TransitionPath = append(sess.TransitionPath, ports.TransitionStep{From: sess.LastCapability, To: pl.Capability})
	}
	sess.Plan.MarkDone(pl.StepID)
	sess.LastCapability, sess.LastOutcome = pl.Capability, outcome
	if err := x.Sessions.Put(sess); err != nil {
		return nil, err
	}
	return []ports.Event{{Type: ports.EvStepCompleted, Payload: ports.PayloadStepCompleted{SessionID: pl.SessionID}}}, nil
}

// handleCapChainToolFailure runs when a tool in a multi-tool capability chain fails.
// Always returns a non-nil event slice or an error (caller returns immediately).
func (x *StepCritic) handleCapChainToolFailure(ctx context.Context, sess *ports.Session, pl ports.PayloadObservation) ([]ports.Event, error) {
	nCap := x.bumpCapabilityFailCount(sess, pl.StepID, pl.Capability)
	if nCap >= x.maxCapabilityFails() {
		if alt := x.trySwitchCapability(ctx, sess, pl.Capability); alt != "" {
			outcome := 1
			if x.RouteGraph != nil {
				if err := x.RouteGraph.RecordTransition(ctx, ports.RoutingContext{IntentText: sess.UserInput}, sess.LastCapability, pl.Capability, outcome); err != nil {
					return nil, err
				}
			}
			sess.TransitionPath = append(sess.TransitionPath, ports.TransitionStep{From: sess.LastCapability, To: pl.Capability})
			sess.LastStepDecision = &ports.Decision{Action: ports.ActionSwitchCapability, Reason: "capability_fail_threshold, switch"}
			sess.CapChainStepID, sess.CapChainTools, sess.CapChainNext = "", nil, 0
			sess.LastCapability, sess.LastOutcome = pl.Capability, outcome
			sess.Plan.MarkPending(pl.StepID)
			if err := x.Sessions.Put(sess); err != nil {
				return nil, err
			}
			return []ports.Event{{Type: ports.EvStepCapabilityRetry, Payload: ports.PayloadStepCapabilityRetry{SessionID: pl.SessionID, StepID: pl.StepID, ExcludeCapability: pl.Capability}}}, nil
		}
	}
	if x.shouldRetryTool(sess, pl) {
		sess.LastStepDecision = &ports.Decision{Action: ports.ActionRetry, Reason: "tool failed, retry"}
		if err := x.Sessions.Put(sess); err != nil {
			return nil, err
		}
		return []ports.Event{{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{SessionID: pl.SessionID, StepID: pl.StepID, Capability: pl.Capability, Tool: pl.Tool, Arguments: planStepArgsForTool(sess, pl.StepID)}}}, nil
	}
	outcome := 1
	if x.RouteGraph != nil {
		if err := x.RouteGraph.RecordTransition(ctx, ports.RoutingContext{IntentText: sess.UserInput}, sess.LastCapability, pl.Capability, outcome); err != nil {
			return nil, err
		}
	}
	sess.TransitionPath = append(sess.TransitionPath, ports.TransitionStep{From: sess.LastCapability, To: pl.Capability})
	if alt := x.trySwitchCapability(ctx, sess, pl.Capability); alt != "" {
		sess.LastStepDecision = &ports.Decision{Action: ports.ActionSwitchCapability, Reason: "cap chain fail, try other capability"}
		sess.CapChainStepID, sess.CapChainTools, sess.CapChainNext = "", nil, 0
		sess.LastCapability, sess.LastOutcome = pl.Capability, outcome
		sess.Plan.MarkPending(pl.StepID)
		if err := x.Sessions.Put(sess); err != nil {
			return nil, err
		}
		return []ports.Event{{Type: ports.EvStepCapabilityRetry, Payload: ports.PayloadStepCapabilityRetry{SessionID: pl.SessionID, StepID: pl.StepID, ExcludeCapability: pl.Capability}}}, nil
	} else {
		sess.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "cap chain fail, mark done"}
		sess.CapChainStepID, sess.CapChainTools, sess.CapChainNext = "", nil, 0
		sess.LastCapability, sess.LastOutcome = pl.Capability, outcome
		sess.Plan.MarkDone(pl.StepID)
		if err := x.Sessions.Put(sess); err != nil {
			return nil, err
		}
		return []ports.Event{{Type: ports.EvStepCompleted, Payload: ports.PayloadStepCompleted{SessionID: pl.SessionID}}}, nil
	}
}

// handleSingleToolFailure runs when the step uses a single tool (or no multi-tool chain) and the call failed.
// Returns (nil, nil) if the caller should continue to the common step-completion path (mark done + graph).
func (x *StepCritic) handleSingleToolFailure(ctx context.Context, sess *ports.Session, pl ports.PayloadObservation) ([]ports.Event, error) {
	nCap := x.bumpCapabilityFailCount(sess, pl.StepID, pl.Capability)
	if nCap >= x.maxCapabilityFails() {
		if alt := x.trySwitchCapability(ctx, sess, pl.Capability); alt != "" {
			sess.LastStepDecision = &ports.Decision{Action: ports.ActionSwitchCapability, Reason: "capability_fail_threshold, switch"}
			sess.LastCapability, sess.LastOutcome = pl.Capability, 1
			sess.Plan.MarkPending(pl.StepID)
			if err := x.Sessions.Put(sess); err != nil {
				return nil, err
			}
			return []ports.Event{{Type: ports.EvStepCapabilityRetry, Payload: ports.PayloadStepCapabilityRetry{SessionID: pl.SessionID, StepID: pl.StepID, ExcludeCapability: pl.Capability}}}, nil
		}
	}
	if x.shouldRetryTool(sess, pl) {
		sess.LastStepDecision = &ports.Decision{Action: ports.ActionRetry, Reason: "tool failed, retry"}
		if err := x.Sessions.Put(sess); err != nil {
			return nil, err
		}
		return []ports.Event{{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{SessionID: pl.SessionID, StepID: pl.StepID, Capability: pl.Capability, Tool: pl.Tool, Arguments: planStepArgsForTool(sess, pl.StepID)}}}, nil
	}
	if alt := x.trySwitchCapability(ctx, sess, pl.Capability); alt != "" {
		sess.LastStepDecision = &ports.Decision{Action: ports.ActionSwitchCapability, Reason: "single tool fail, try other capability"}
		sess.LastCapability, sess.LastOutcome = pl.Capability, 1
		sess.Plan.MarkPending(pl.StepID)
		if err := x.Sessions.Put(sess); err != nil {
			return nil, err
		}
		return []ports.Event{{Type: ports.EvStepCapabilityRetry, Payload: ports.PayloadStepCapabilityRetry{SessionID: pl.SessionID, StepID: pl.StepID, ExcludeCapability: pl.Capability}}}, nil
	} else {
		sess.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "single tool fail, mark done"}
		return nil, nil
	}
}

func (x *StepCritic) shouldRetryTool(sess *ports.Session, pl ports.PayloadObservation) bool {
	if pl.Obs.(ports.Observation).Err == nil {
		return false
	}
	if sess.ToolFailCount == nil {
		sess.ToolFailCount = make(map[string]int)
	}
	k := rtutils.ToolFailCountKey(pl.StepID, pl.Tool)
	sess.ToolFailCount[k]++
	return sess.ToolFailCount[k] < x.maxStepToolFails()
}

func planStepArgsForTool(sess *ports.Session, stepID string) map[string]any {
	a := skill.RenderPlanStepArguments(sess, stepID)
	if a == nil {
		return rtutils.PlanStepArguments(sess, stepID)
	}
	return a
}

func (x *StepCritic) trySwitchCapability(ctx context.Context, sess *ports.Session, exclude string) string {
	if x.RouteGraph == nil || x.CapRegistry == nil {
		return ""
	}
	rc := ports.RoutingContext{IntentText: sess.UserInput}
	frontier, _ := x.RouteGraph.Frontier(ctx, rc, sess.LastCapability, 1)
	if len(frontier) == 0 {
		frontier, _ = x.RouteGraph.EntryNodes(ctx, rc)
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
			return capID
		}
	}
	return ""
}
