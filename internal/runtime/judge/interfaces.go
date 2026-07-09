package judge

import "github.com/OctoSucker/octosucker/internal/runtime/toolrouting"

type TransitionRecorder interface {
	RecordTransition(intent string, cost, latency float64, from, to toolrouting.Node, success bool) error
}
