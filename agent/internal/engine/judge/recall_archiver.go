package judge

import (
	"context"
	"strings"

	"github.com/OctoSucker/agent/internal/store/recall"
	"github.com/OctoSucker/agent/internal/store/task"
	"github.com/OctoSucker/agent/pkg/ports"
)

type RecallArchiver struct {
	Tasks  *task.TaskStore
	Recall *recall.RecallCorpus
}

func (r *RecallArchiver) ArchiveRecall(ctx context.Context, evt ports.Event) error {
	pl := evt.Payload.(ports.PayloadTurnFinalized)
	taskState, ok := r.Tasks.Get(pl.TaskID)
	if !ok {
		return nil
	}
	doc := strings.TrimSpace(taskState.Reply)
	if ts := strings.TrimSpace(taskState.TrajectorySummary); ts != "" {
		if doc != "" {
			doc += "\n\n" + ts
		} else {
			doc = ts
		}
	}
	if doc == "" {
		return nil
	}
	return r.Recall.Write(ctx, "task="+pl.TaskID+" "+doc)
}
