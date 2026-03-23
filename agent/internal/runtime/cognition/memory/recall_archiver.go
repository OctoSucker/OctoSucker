package memory

import (
	"context"

	"github.com/OctoSucker/agent/pkg/ports"
)

type SessionRepository interface {
	Get(id string) (*ports.Session, bool)
}

type RecallStore interface {
	Write(ctx context.Context, text string) error
}

type RecallArchiver struct {
	Sessions SessionRepository
	Recall   RecallStore
}

func (r *RecallArchiver) ArchiveRecall(ctx context.Context, evt ports.Event) error {
	pl := evt.Payload.(ports.PayloadTurnFinalized)
	sess, ok := r.Sessions.Get(pl.SessionID)
	if !ok || sess.Reply == "" {
		return nil
	}
	return r.Recall.Write(ctx, "session="+pl.SessionID+" "+sess.Reply)
}
