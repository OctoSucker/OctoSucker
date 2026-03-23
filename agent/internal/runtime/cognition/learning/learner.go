package learning

import (
	"context"

	"github.com/OctoSucker/agent/internal/runtime/store"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

type Learner struct {
	Sessions              *store.SessionStore
	Skills                *store.SkillRegistry
	RouteGraph            *store.RoutingGraph
	Embedder              *llmclient.OpenAI
	SkillRouteThreshold   float64
	ExtractScoreThreshold float64
}

func (l *Learner) RecordSkillLearning(ctx context.Context, evt ports.Event) error {
	pl := evt.Payload.(ports.PayloadTurnFinalized)
	sess, ok := l.Sessions.Get(pl.SessionID)
	if !ok {
		return nil
	}
	success := sess.TrajectoryScore >= 0.5
	var emb []float32
	if l.Embedder != nil && sess.UserInput != "" {
		var err error
		emb, err = l.Embedder.Embed(ctx, sess.UserInput)
		if err != nil {
			emb = nil
		}
	}
	l.Skills.RecordTurn(sess.UserInput, success, emb, l.SkillRouteThreshold, sess.ActiveSkillName, sess.ActiveSkillVariantID)
	if len(sess.TransitionPath) > 0 {
		l.RouteGraph.RecordTrajectory(sess.TransitionPath, float64(sess.TrajectoryScore), success)
	}
	if success && float64(sess.TrajectoryScore) >= l.ExtractScoreThreshold {
		if entry, ok := store.BuildSkillEntryFromSession(ctx, sess, l.Embedder); ok {
			l.Skills.MergeOrAdd(entry)
		}
	}
	return nil
}
