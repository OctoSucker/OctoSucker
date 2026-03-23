package engine

import (
	"context"
	"maps"

	"github.com/OctoSucker/agent/pkg/ports"
)

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

func failKey(stepID, tool string) string {
	return stepID + "\x1e" + tool
}

func (d *Dispatcher) maxStepToolFails() int {
	if d.MaxFailsPerTool <= 0 {
		panic("engine.Dispatcher: MaxFailsPerTool must be > 0")
	}
	return d.MaxFailsPerTool
}

func (d *Dispatcher) stepCriticObservationReady(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	pl := evt.Payload.(ports.PayloadObservation)
	obs := pl.Obs.(ports.Observation)
	sess, ok := d.Sessions.Get(pl.SessionID)
	if !ok {
		return nil, nil
	}
	sess.StepID = pl.StepID
	sess.Trace = append(sess.Trace, ports.StepTrace{
		StepID: pl.StepID, Tool: pl.Tool, OK: obs.Err == nil, Summary: obs.Summary,
	})
	if obs.Err == nil {
		if sess.ToolFailCount != nil {
			delete(sess.ToolFailCount, failKey(pl.StepID, pl.Tool))
		}
	}
	if pl.StepID == sess.CapChainStepID && len(sess.CapChainTools) > 1 {
		if obs.Err != nil {
			if d.shouldRetryTool(sess, pl) {
				sess.LastStepDecision = &ports.Decision{Action: ports.ActionRetry, Reason: "tool failed, retry"}
				if err := d.Sessions.Put(sess); err != nil {
					return nil, err
				}
				return []ports.Event{{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{
					SessionID: pl.SessionID, StepID: pl.StepID, Capability: pl.Capability, Tool: pl.Tool,
					Arguments: stepArgumentsFromPlan(sess, pl.StepID),
				}}}, nil
			}
			outcome := 1
			err := d.RouteGraph.RecordTransition(ctx, ports.RoutingContext{IntentText: sess.UserInput}, sess.LastCapability, pl.Capability, outcome)
			if err != nil {
				return nil, err
			}
			sess.TransitionPath = append(sess.TransitionPath, ports.TransitionStep{From: sess.LastCapability, To: pl.Capability})
			if alt := d.trySwitchCapability(ctx, sess, pl.Capability); alt != "" {
				sess.LastStepDecision = &ports.Decision{Action: ports.ActionSwitchCapability, Reason: "cap chain fail, try other capability"}
				sess.CapChainStepID = ""
				sess.CapChainTools = nil
				sess.CapChainNext = 0
				sess.LastCapability = pl.Capability
				sess.LastOutcome = outcome
				sess.Plan.MarkPending(pl.StepID)
				if err := d.Sessions.Put(sess); err != nil {
					return nil, err
				}
				return []ports.Event{{Type: ports.EvStepCapabilityRetry, Payload: ports.PayloadStepCapabilityRetry{
					SessionID: pl.SessionID, StepID: pl.StepID, ExcludeCapability: pl.Capability,
				}}}, nil
			}
			sess.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "cap chain fail, mark done"}
			sess.CapChainStepID = ""
			sess.CapChainTools = nil
			sess.CapChainNext = 0
			sess.LastCapability = pl.Capability
			sess.LastOutcome = outcome
			sess.Plan.MarkDone(pl.StepID)
			if err := d.Sessions.Put(sess); err != nil {
				return nil, err
			}
			return []ports.Event{{Type: ports.EvStepCompleted, Payload: ports.PayloadStepCompleted{SessionID: pl.SessionID}}}, nil
		}
		sess.CapChainNext++
		if sess.CapChainNext < len(sess.CapChainTools) {
			sess.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "next in chain"}
			sess.PendingTool = sess.CapChainTools[sess.CapChainNext]
			if err := d.Sessions.Put(sess); err != nil {
				return nil, err
			}
			return []ports.Event{{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{
				SessionID: pl.SessionID, StepID: pl.StepID, Capability: pl.Capability, Tool: sess.PendingTool,
				Arguments: stepArgumentsFromPlan(sess, pl.StepID),
			}}}, nil
		}
		sess.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "chain done"}
		sess.CapChainStepID = ""
		sess.CapChainTools = nil
		sess.CapChainNext = 0
	} else if obs.Err != nil && len(sess.CapChainTools) <= 1 {
		if d.shouldRetryTool(sess, pl) {
			sess.LastStepDecision = &ports.Decision{Action: ports.ActionRetry, Reason: "tool failed, retry"}
			if err := d.Sessions.Put(sess); err != nil {
				return nil, err
			}
			return []ports.Event{{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{
				SessionID: pl.SessionID, StepID: pl.StepID, Capability: pl.Capability, Tool: pl.Tool,
				Arguments: stepArgumentsFromPlan(sess, pl.StepID),
			}}}, nil
		}
		if alt := d.trySwitchCapability(ctx, sess, pl.Capability); alt != "" {
			sess.LastStepDecision = &ports.Decision{Action: ports.ActionSwitchCapability, Reason: "single tool fail, try other capability"}
			sess.LastCapability = pl.Capability
			sess.LastOutcome = 1
			sess.Plan.MarkPending(pl.StepID)
			if err := d.Sessions.Put(sess); err != nil {
				return nil, err
			}
			return []ports.Event{{Type: ports.EvStepCapabilityRetry, Payload: ports.PayloadStepCapabilityRetry{
				SessionID: pl.SessionID, StepID: pl.StepID, ExcludeCapability: pl.Capability,
			}}}, nil
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
	if d.RouteGraph != nil {
		_ = d.RouteGraph.RecordTransition(ctx, ports.RoutingContext{IntentText: sess.UserInput}, sess.LastCapability, pl.Capability, outcome)
		sess.TransitionPath = append(sess.TransitionPath, ports.TransitionStep{From: sess.LastCapability, To: pl.Capability})
	}
	sess.Plan.MarkDone(pl.StepID)
	sess.LastCapability = pl.Capability
	sess.LastOutcome = outcome
	if err := d.Sessions.Put(sess); err != nil {
		return nil, err
	}
	return []ports.Event{{Type: ports.EvStepCompleted, Payload: ports.PayloadStepCompleted{SessionID: pl.SessionID}}}, nil
}

func (d *Dispatcher) shouldRetryTool(sess *ports.Session, pl ports.PayloadObservation) bool {
	if pl.Obs.(ports.Observation).Err == nil {
		return false
	}
	if sess.ToolFailCount == nil {
		sess.ToolFailCount = make(map[string]int)
	}
	k := failKey(pl.StepID, pl.Tool)
	sess.ToolFailCount[k]++
	return sess.ToolFailCount[k] < d.maxStepToolFails()
}

func (d *Dispatcher) trySwitchCapability(ctx context.Context, sess *ports.Session, exclude string) string {
	if d.RouteGraph == nil || d.CapRegistry == nil {
		return ""
	}
	rc := ports.RoutingContext{IntentText: sess.UserInput}
	frontier, _ := d.RouteGraph.Frontier(ctx, rc, sess.LastCapability, 1)
	if len(frontier) == 0 {
		frontier, _ = d.RouteGraph.EntryNodes(ctx, rc)
	}
	allow := make(map[string]bool)
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
		if d.CapRegistry.FirstTool(capID) != "" {
			return capID
		}
	}
	return ""
}
