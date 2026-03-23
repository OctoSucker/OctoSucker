package execution

import (
	"context"

	"github.com/OctoSucker/agent/pkg/ports"
)

type FullSessionRepository interface {
	Get(id string) (*ports.Session, bool)
	Put(sess *ports.Session) error
}

type RouteGraph interface {
	RecordTransition(ctx context.Context, rc ports.RoutingContext, from, to string, outcome int) error
	Frontier(ctx context.Context, rc ports.RoutingContext, last string, outcome int) ([]string, error)
	EntryNodes(ctx context.Context, rc ports.RoutingContext) ([]string, error)
}

type CapabilityRegistry interface {
	FirstTool(capID string) string
}
