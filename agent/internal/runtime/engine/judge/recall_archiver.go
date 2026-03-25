package judge

import (
	"context"

	"github.com/OctoSucker/agent/internal/runtime/store/recall"
	"github.com/OctoSucker/agent/internal/runtime/store/task"
	"github.com/OctoSucker/agent/pkg/ports"
)

type RecallArchiver struct {
	Tasks  *task.TaskStore
	Recall *recall.RecallCorpus
}

func (r *RecallArchiver) ArchiveRecall(ctx context.Context, evt ports.Event) error {
	pl := evt.Payload.(ports.PayloadTurnFinalized)
	sess, ok := r.Tasks.Get(pl.TaskID)
	if !ok || sess.Reply == "" {
		return nil
	}
	return r.Recall.Write(ctx, "task="+pl.TaskID+" "+sess.Reply)
}
