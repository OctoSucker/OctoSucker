package engine

import (
	"context"

	"github.com/OctoSucker/agent/pkg/ports"
)

func (d *Dispatcher) archiveRecall(ctx context.Context, evt ports.Event) error {
	pl := evt.Payload.(ports.PayloadTurnFinalized)
	sess, ok := d.Sessions.Get(pl.SessionID)
	if !ok || sess.Reply == "" {
		return nil
	}
	return d.RecallCorpus.Write(ctx, "session="+pl.SessionID+" "+sess.Reply)
}
