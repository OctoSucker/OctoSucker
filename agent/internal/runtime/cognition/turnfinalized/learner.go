package turnfinalized

import (
	"context"
	"database/sql"
	"log"

	routinggraph "github.com/OctoSucker/agent/internal/runtime/store/routing_graph"
	"github.com/OctoSucker/agent/internal/runtime/store/session"
	skill "github.com/OctoSucker/agent/internal/runtime/store/skill"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

type Learner struct {
	Sessions              *session.SessionStore
	Skills                *skill.SkillRegistry
	RouteGraph            *routinggraph.RoutingGraph
	Embedder              *llmclient.OpenAI
	SkillRouteThreshold   float64
	ExtractScoreThreshold float64
	// SQLDB backs skill_learn_progress (N qualifying successes per cap path). Nil disables counter and restores immediate extract on qualify.
	SQLDB *sql.DB
	// MinPlanStepsForSkillExtract: require at least this many plan steps before counting / extracting (0 = no minimum).
	MinPlanStepsForSkillExtract int
	// MinQualifyingSuccessesForSkill: cumulative qualifying successes per cap_key before MergeOrAdd; <=0 or 1 with DB acts like first success extracts.
	MinQualifyingSuccessesForSkill int
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
		l.maybeExtractSkillFromSession(ctx, sess)
	}
	return nil
}

// maybeExtractSkillFromSession applies min plan steps + N qualifying successes (per capability path) before MergeOrAdd.
func (l *Learner) maybeExtractSkillFromSession(ctx context.Context, sess *ports.Session) {
	if l == nil || sess == nil {
		return
	}
	capKey, nSteps, ok := skill.SkillLearnCapKeyFromSession(sess)
	if !ok {
		return
	}
	if l.MinPlanStepsForSkillExtract > 0 && nSteps < l.MinPlanStepsForSkillExtract {
		return
	}
	nNeed := l.MinQualifyingSuccessesForSkill
	if nNeed <= 0 {
		nNeed = 1
	}
	c, err := skill.BumpSkillLearnSuccessCount(l.SQLDB, capKey)
	if err != nil {
		log.Printf("turnfinalized.Learner: bump skill learn progress cap_key=%s err=%v", capKey, err)
		return
	}
	if c < nNeed {
		return
	}
	entry, ok := skill.BuildSkillEntryFromSession(ctx, sess, l.Embedder)
	if !ok {
		return
	}
	l.Skills.MergeOrAdd(entry)
	if err := skill.ResetSkillLearnSuccessCount(l.SQLDB, capKey); err != nil {
		log.Printf("turnfinalized.Learner: reset skill learn progress cap_key=%s err=%v", capKey, err)
	}
}
