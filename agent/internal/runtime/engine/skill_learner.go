package engine

import (
	"context"

	"github.com/OctoSucker/agent/internal/runtime/store"
	"github.com/OctoSucker/agent/pkg/ports"
)

func (d *Dispatcher) recordSkillLearning(ctx context.Context, evt ports.Event) error {
	pl := evt.Payload.(ports.PayloadTurnFinalized)
	sess, ok := d.Sessions.Get(pl.SessionID)
	if !ok {
		return nil
	}
	success := sess.TrajectoryScore >= 0.5
	var emb []float32
	if d.Embedder != nil && sess.UserInput != "" {
		var err error
		emb, err = d.Embedder.Embed(ctx, sess.UserInput)
		if err != nil {
			emb = nil
		}
	}
	d.Skills.RecordTurn(sess.UserInput, success, emb, d.SkillRouteThreshold)
	if len(sess.TransitionPath) > 0 {
		d.RouteGraph.RecordTrajectory(sess.TransitionPath, float64(sess.TrajectoryScore), success)
	}
	if success && float64(sess.TrajectoryScore) >= d.ExtractScoreThreshold {
		if entry, ok := store.BuildSkillEntryFromSession(ctx, sess, d.Embedder); ok {
			d.Skills.MergeOrAdd(entry)
		}
	}
	return nil
}
