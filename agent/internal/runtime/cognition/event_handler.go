package cognition

import (
	"context"

	"github.com/OctoSucker/agent/pkg/ports"
)

// EventHandler is the function shape for runtime event dispatch (planner, critics, executors, etc.).
type EventHandler func(context.Context, ports.Event) ([]ports.Event, error)
