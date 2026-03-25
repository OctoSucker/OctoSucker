package judge

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	routinggraph "github.com/OctoSucker/agent/internal/runtime/store/routing_graph"
	skill "github.com/OctoSucker/agent/internal/runtime/store/skill"
	"github.com/OctoSucker/agent/internal/runtime/store/task"
	"github.com/OctoSucker/agent/pkg/ports"
)

type Learner struct {
	Tasks                 *task.TaskStore
	Skills                *skill.SkillRegistry
	RouteGraph            *routinggraph.RoutingGraph
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
	taskState, ok := l.Tasks.Get(pl.TaskID)
	if !ok {
		return fmt.Errorf("turnfinalized: task not found: %s", pl.TaskID)
	}
	success := taskState.TrajectoryScore >= 0.5
	emb, err := l.Skills.EmbedText(ctx, taskState.UserInput.Text)
	if err != nil {
		return err
	}
	if err := l.Skills.RecordTurn(taskState.UserInput.Text, success, emb, l.SkillRouteThreshold, taskState.ActiveSkillName, taskState.ActiveSkillVariantID); err != nil {
		return err
	}
	if len(taskState.TransitionPath) > 0 {
		if err := l.RouteGraph.RecordTrajectory(taskState.TransitionPath, float64(taskState.TrajectoryScore), success); err != nil {
			return fmt.Errorf("turnfinalized: record trajectory: %w", err)
		}
	}
	if success && float64(taskState.TrajectoryScore) >= l.ExtractScoreThreshold {
		if err := l.maybeExtractSkillFromTask(ctx, taskState); err != nil {
			return err
		}
	}
	return nil
}

// maybeExtractSkillFromTask applies min plan steps + N qualifying successes (per capability path) before MergeOrAdd.
func (l *Learner) maybeExtractSkillFromTask(ctx context.Context, taskState *ports.Task) error {
	if taskState == nil {
		return nil
	}
	capKey, nSteps, ok := skill.SkillLearnCapKeyFromTask(taskState)
	if !ok {
		return nil
	}
	if l.MinPlanStepsForSkillExtract > 0 && nSteps < l.MinPlanStepsForSkillExtract {
		return nil
	}
	nNeed := l.MinQualifyingSuccessesForSkill
	if nNeed <= 0 {
		nNeed = 1
	}
	c, err := skill.BumpSkillLearnSuccessCount(l.SQLDB, capKey)
	if err != nil {
		return fmt.Errorf("turnfinalized: bump skill learn progress cap_key=%s: %w", capKey, err)
	}
	if c < nNeed {
		return nil
	}
	entry, err := l.Skills.BuildEntryFromTask(ctx, taskState)
	if errors.Is(err, skill.ErrNoSkillFromTask) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := l.Skills.MergeOrAdd(entry); err != nil {
		return err
	}
	if err := skill.ResetSkillLearnSuccessCount(l.SQLDB, capKey); err != nil {
		return fmt.Errorf("turnfinalized: reset skill learn progress cap_key=%s: %w", capKey, err)
	}
	return nil
}
