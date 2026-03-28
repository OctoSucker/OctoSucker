package judge

import (
	"context"
	"errors"
	"fmt"

	procedure "github.com/OctoSucker/agent/internal/store/procedure"
	routinggraph "github.com/OctoSucker/agent/internal/store/routing_graph"
	"github.com/OctoSucker/agent/internal/store/task"
	"github.com/OctoSucker/agent/pkg/ports"
)

const procedureRouteThreshold = 0.9
const extractScoreThreshold = 0.8
const minPlanStepsForProcedureExtract = 3
const minQualifyingSuccessesForProcedure = 2
const successThreshold = 0.5

type Learner struct {
	Tasks      *task.TaskStore
	Procedures *procedure.ProcedureRegistry
	RouteGraph *routinggraph.RoutingGraph
}

func (l *Learner) RecordProcedureLearning(ctx context.Context, evt ports.Event) error {
	pl := evt.Payload.(ports.PayloadTurnFinalized)
	taskState, ok := l.Tasks.Get(pl.TaskID)
	if !ok {
		return fmt.Errorf("turnfinalized: task not found: %s", pl.TaskID)
	}
	success := taskState.TrajectoryScore >= successThreshold
	emb, err := l.Procedures.EmbedText(ctx, taskState.UserInput.Text)
	if err != nil {
		return err
	}
	if err := l.Procedures.RecordTurn(taskState.UserInput.Text, success, emb, procedureRouteThreshold, taskState.ActiveProcedureName, taskState.ActiveProcedureVariantID); err != nil {
		return err
	}
	if success && taskState.ActiveProcedureName != "" && taskState.ActiveProcedureVariantID != "" {
		if err := l.Procedures.MarkUsed(taskState.ActiveProcedureName, taskState.ActiveProcedureVariantID); err != nil {
			return fmt.Errorf("turnfinalized: mark procedure used: %w", err)
		}
	}
	if len(taskState.TransitionPath) > 0 {
		if err := l.RouteGraph.RecordTrajectory(taskState.TransitionPath, taskState.TrajectoryScore, success); err != nil {
			return fmt.Errorf("turnfinalized: record trajectory: %w", err)
		}
	}
	if success && float64(taskState.TrajectoryScore) >= extractScoreThreshold {
		if err := l.maybeExtractProcedureFromTask(ctx, taskState); err != nil {
			return err
		}
	}
	return nil
}

// maybeExtractProcedureFromTask applies min plan steps + N qualifying successes (per capability path) before MergeOrAdd.
func (l *Learner) maybeExtractProcedureFromTask(ctx context.Context, taskState *ports.Task) error {
	if taskState == nil {
		return nil
	}
	capKey, nSteps, ok := procedure.ProcedureLearnCapKeyFromTask(taskState)
	if !ok {
		return nil
	}
	if minPlanStepsForProcedureExtract > 0 && nSteps < minPlanStepsForProcedureExtract {
		return nil
	}
	nNeed := minQualifyingSuccessesForProcedure
	if nNeed <= 0 {
		nNeed = 1
	}
	c, err := l.Procedures.BumpProcedureLearnSuccessCount(capKey)
	if err != nil {
		return fmt.Errorf("turnfinalized: bump procedure learn progress cap_key=%s: %w", capKey, err)
	}
	if c < nNeed {
		return nil
	}
	entry, err := l.Procedures.BuildEntryFromTask(ctx, taskState)
	if errors.Is(err, procedure.ErrNoProcedureFromTask) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := l.Procedures.MergeOrAdd(entry); err != nil {
		return err
	}
	if err := l.Procedures.ResetProcedureLearnSuccessCount(capKey); err != nil {
		return fmt.Errorf("turnfinalized: reset procedure learn progress cap_key=%s: %w", capKey, err)
	}
	return nil
}
