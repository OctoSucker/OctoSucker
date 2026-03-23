package evaluation

import (
	"context"
	"maps"

	"github.com/OctoSucker/agent/pkg/ports"
)

type FullSessionRepository interface {
	Get(id string) (*ports.Session, bool)
	Put(sess *ports.Session) error
}

type RouteGraph interface {
	RecordTransition(ctx context.Context, rc ports.RoutingContext, from, to string, outcome int) error
	Frontier(ctx context.Context, rc ports.RoutingContext, last string, outcome int) ([]string, error)
	EntryNodes(ctx context.Context, rc ports.RoutingContext) ([]string, error)
}

type CapabilityRegistry interface {
	FirstTool(capID string) string
}

type StepCritic struct {
	Sessions        FullSessionRepository
	RouteGraph      RouteGraph
	CapRegistry     CapabilityRegistry
	MaxFailsPerTool int
}

func stepArgumentsFromPlan(sess *ports.Session, stepID string) map[string]any {
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

func failKey(stepID, tool string) string { return stepID + "\x1e" + tool }

func (x *StepCritic) maxStepToolFails() int {
	if x.MaxFailsPerTool <= 0 {
		panic("evaluation.StepCritic: MaxFailsPerTool must be > 0")
	}
	return x.MaxFailsPerTool
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
	if obs.Err == nil && sess.ToolFailCount != nil {
		delete(sess.ToolFailCount, failKey(pl.StepID, pl.Tool))
	}
	if pl.StepID == sess.CapChainStepID && len(sess.CapChainTools) > 1 {
		if obs.Err != nil {
			if x.shouldRetryTool(sess, pl) {
				sess.LastStepDecision = &ports.Decision{Action: ports.ActionRetry, Reason: "tool failed, retry"}
				if err := x.Sessions.Put(sess); err != nil {
					return nil, err
				}
				return []ports.Event{{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{SessionID: pl.SessionID, StepID: pl.StepID, Capability: pl.Capability, Tool: pl.Tool, Arguments: stepArgumentsFromPlan(sess, pl.StepID)}}}, nil
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
			}
			sess.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "cap chain fail, mark done"}
			sess.CapChainStepID, sess.CapChainTools, sess.CapChainNext = "", nil, 0
			sess.LastCapability, sess.LastOutcome = pl.Capability, outcome
			sess.Plan.MarkDone(pl.StepID)
			if err := x.Sessions.Put(sess); err != nil {
				return nil, err
			}
			return []ports.Event{{Type: ports.EvStepCompleted, Payload: ports.PayloadStepCompleted{SessionID: pl.SessionID}}}, nil
		}
		sess.CapChainNext++
		if sess.CapChainNext < len(sess.CapChainTools) {
			sess.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "next in chain"}
			sess.PendingTool = sess.CapChainTools[sess.CapChainNext]
			if err := x.Sessions.Put(sess); err != nil {
				return nil, err
			}
			return []ports.Event{{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{SessionID: pl.SessionID, StepID: pl.StepID, Capability: pl.Capability, Tool: sess.PendingTool, Arguments: stepArgumentsFromPlan(sess, pl.StepID)}}}, nil
		}
		sess.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "chain done"}
		sess.CapChainStepID, sess.CapChainTools, sess.CapChainNext = "", nil, 0
	} else if obs.Err != nil && len(sess.CapChainTools) <= 1 {
		if x.shouldRetryTool(sess, pl) {
			sess.LastStepDecision = &ports.Decision{Action: ports.ActionRetry, Reason: "tool failed, retry"}
			if err := x.Sessions.Put(sess); err != nil {
				return nil, err
			}
			return []ports.Event{{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{SessionID: pl.SessionID, StepID: pl.StepID, Capability: pl.Capability, Tool: pl.Tool, Arguments: stepArgumentsFromPlan(sess, pl.StepID)}}}, nil
		}
		if alt := x.trySwitchCapability(ctx, sess, pl.Capability); alt != "" {
			sess.LastStepDecision = &ports.Decision{Action: ports.ActionSwitchCapability, Reason: "single tool fail, try other capability"}
			sess.LastCapability, sess.LastOutcome = pl.Capability, 1
			sess.Plan.MarkPending(pl.StepID)
			if err := x.Sessions.Put(sess); err != nil {
				return nil, err
			}
			return []ports.Event{{Type: ports.EvStepCapabilityRetry, Payload: ports.PayloadStepCapabilityRetry{SessionID: pl.SessionID, StepID: pl.StepID, ExcludeCapability: pl.Capability}}}, nil
		}
		sess.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "single tool fail, mark done"}
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

func (x *StepCritic) shouldRetryTool(sess *ports.Session, pl ports.PayloadObservation) bool {
	if pl.Obs.(ports.Observation).Err == nil {
		return false
	}
	if sess.ToolFailCount == nil {
		sess.ToolFailCount = make(map[string]int)
	}
	k := failKey(pl.StepID, pl.Tool)
	sess.ToolFailCount[k]++
	return sess.ToolFailCount[k] < x.maxStepToolFails()
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
