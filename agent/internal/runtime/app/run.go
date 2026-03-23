package app

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/OctoSucker/agent/internal/runtime/store"
	"github.com/OctoSucker/agent/pkg/ports"
)

var ErrRerunNoPlan = errors.New("session or plan missing")

func (a *App) RunInput(ctx context.Context, sessionID, text string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session id required")
	}
	q := []ports.Event{{Type: ports.EvUserInput, Payload: ports.PayloadUserInput{SessionID: sessionID, Text: text}}}
	replies, err := a.RunEvents(ctx, q)
	if err != nil {
		return "", err
	}
	r, ok := replies[sessionID]
	if !ok {
		return "", fmt.Errorf("session missing")
	}
	return r, nil
}

func (a *App) RunEvents(ctx context.Context, queue []ports.Event) (map[string]string, error) {
	if err := a.Dispatcher.Run(ctx, queue); err != nil {
		log.Printf("app.RunEvents: dispatcher error err=%v", err)
		return nil, err
	}
	seen := make(map[string]struct{})
	for _, e := range queue {
		if e.Type != ports.EvUserInput {
			continue
		}
		p, ok := e.Payload.(ports.PayloadUserInput)
		if !ok {
			continue
		}
		if _, dup := seen[p.SessionID]; dup {
			continue
		}
		seen[p.SessionID] = struct{}{}
	}
	out := make(map[string]string)
	for sid := range seen {
		if r, ok := replyFromStore(a.Dispatcher.Sessions, sid); ok {
			out[sid] = r
		}
	}
	return out, nil
}

func replyFromStore(s *store.SessionStore, sessionID string) (string, bool) {
	if s == nil || sessionID == "" {
		return "", false
	}
	sess, ok := s.Get(sessionID)
	if !ok || sess == nil {
		return "", false
	}
	return sess.Reply, true
}

func (a *App) RerunSessionPlan(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session id required")
	}
	if a == nil || a.Dispatcher == nil || a.Dispatcher.Sessions == nil {
		return "", fmt.Errorf("app: nil dispatcher or sessions")
	}
	sessStore := a.Dispatcher.Sessions
	sess, ok := sessStore.Get(sessionID)
	if !ok || sess == nil || sess.Plan == nil {
		return "", ErrRerunNoPlan
	}
	for i := range sess.Plan.Steps {
		sess.Plan.Steps[i].Status = "pending"
	}
	sess.Trace = nil
	sess.ToolFailCount = nil
	sess.CapChainStepID = ""
	sess.CapChainTools = nil
	sess.CapChainNext = 0
	sess.StepID = ""
	sess.PendingTool = ""
	sess.LastCapability = ""
	sess.LastOutcome = 0
	sess.Reply = ""
	sess.TrajectoryScore = 0
	sess.TrajectorySummary = ""
	sess.ReplanAllowed = true
	sess.ReplanCount = 0
	if err := sessStore.Put(sess); err != nil {
		return "", err
	}
	q := []ports.Event{{Type: ports.EvPlanCreated, Payload: ports.PayloadPlanCreated{SessionID: sessionID}}}
	if err := a.Dispatcher.Run(ctx, q); err != nil {
		log.Printf("app.RerunSessionPlan: dispatcher error session=%s err=%v", sessionID, err)
		return "", err
	}
	reply, ok := replyFromStore(sessStore, sessionID)
	if !ok {
		return "", fmt.Errorf("session missing")
	}
	return reply, nil
}
