package turnfinalized

import (
	"context"

	"github.com/OctoSucker/agent/internal/runtime/store/recall"
	"github.com/OctoSucker/agent/internal/runtime/store/session"
	"github.com/OctoSucker/agent/pkg/ports"
)

type RecallArchiver struct {
	Sessions *session.SessionStore
	Recall   *recall.RecallCorpus
}

func (r *RecallArchiver) ArchiveRecall(ctx context.Context, evt ports.Event) error {
	pl := evt.Payload.(ports.PayloadTurnFinalized)
	sess, ok := r.Sessions.Get(pl.SessionID)
	if !ok || sess.Reply == "" {
		return nil
	}
	return r.Recall.Write(ctx, "session="+pl.SessionID+" "+sess.Reply)
}
