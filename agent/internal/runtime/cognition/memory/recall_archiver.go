package memory

import (
	"context"

	"github.com/OctoSucker/agent/internal/runtime/store"
	"github.com/OctoSucker/agent/pkg/ports"
)

type RecallArchiver struct {
	Sessions *store.SessionStore
	Recall   *store.RecallCorpus
}

func (r *RecallArchiver) ArchiveRecall(ctx context.Context, evt ports.Event) error {
	pl := evt.Payload.(ports.PayloadTurnFinalized)
	sess, ok := r.Sessions.Get(pl.SessionID)
	if !ok || sess.Reply == "" {
		return nil
	}
	return r.Recall.Write(ctx, "session="+pl.SessionID+" "+sess.Reply)
}
