package engine

import (
	"context"

	"github.com/OctoSucker/agent/internal/runtime/store"
	"github.com/OctoSucker/agent/pkg/ports"
)

type SessionRepository interface {
	Get(id string) (*ports.Session, bool)
	Put(sess *ports.Session) error
}

type RouteGraphStore interface {
	Confidence(ctx context.Context, rc ports.RoutingContext, last string) float64
	Frontier(ctx context.Context, rc ports.RoutingContext, last string, outcome int) ([]string, error)
	EntryNodes(ctx context.Context, rc ports.RoutingContext) ([]string, error)
	RecordTransition(ctx context.Context, rc ports.RoutingContext, from, to string, outcome int) error
	RecordTrajectory(path []ports.TransitionStep, score float64, success bool)
}

type SkillStore interface {
	Match(userText string) []string
	MatchByEmbedding(embedding []float32, k int) []store.SkillEntry
	KeywordPlanEntry(userText string) (store.SkillEntry, bool)
	MarkUsed(name string)
	RecordTurn(userText string, success bool, queryEmbedding []float32, minEmbeddingSim float64, activeSkillName, activeVariantID string)
	MergeOrAdd(e store.SkillEntry)
}

type RecallStore interface {
	Write(ctx context.Context, text string) error
	Recall(ctx context.Context, query string, k int) ([]string, error)
}

type CapabilityStore interface {
	FirstTool(capID string) string
	Tools(capID string) []string
}

type ToolInvoker interface {
	Invoke(ctx context.Context, inv ports.CapabilityInvocation) (ports.ToolResult, error)
}
