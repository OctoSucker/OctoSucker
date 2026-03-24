package turnfinalized

import (
	"context"
	"database/sql"

	"github.com/OctoSucker/agent/internal/runtime/store/recall"
	routinggraph "github.com/OctoSucker/agent/internal/runtime/store/routing_graph"
	"github.com/OctoSucker/agent/internal/runtime/store/session"
	skill "github.com/OctoSucker/agent/internal/runtime/store/skill"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

// Handler runs the EvTurnFinalized pipeline (skill learning, recall archival).
type Handler struct {
	Learner        *Learner
	RecallArchiver *RecallArchiver
}

// NewHandler wires dependencies for Dispatcher registration.
func NewHandler(sessions *session.SessionStore,
	skills *skill.SkillRegistry,
	routeGraph *routinggraph.RoutingGraph,
	embedder *llmclient.OpenAI,
	skillRouteThreshold float64,
	extractScoreThreshold float64,
	sqlDB *sql.DB,
	skillLearnMinPlanSteps int,
	skillLearnMinSuccessCount int,
	recallCorpus *recall.RecallCorpus,
) *Handler {
	learner := &Learner{
		Sessions:                       sessions,
		Skills:                         skills,
		RouteGraph:                     routeGraph,
		Embedder:                       embedder,
		SkillRouteThreshold:            skillRouteThreshold,
		ExtractScoreThreshold:          extractScoreThreshold,
		SQLDB:                          sqlDB,
		MinPlanStepsForSkillExtract:    skillLearnMinPlanSteps,
		MinQualifyingSuccessesForSkill: skillLearnMinSuccessCount,
	}
	archiver := &RecallArchiver{
		Sessions: sessions,
		Recall:   recallCorpus,
	}
	return &Handler{Learner: learner, RecallArchiver: archiver}
}

// HandleTurnFinalized matches cognition.EventHandler (used by engine.Dispatcher).
func (h *Handler) HandleTurnFinalized(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	if h == nil {
		return nil, nil
	}
	if h.Learner == nil || h.RecallArchiver == nil {
		return nil, nil
	}
	if err := h.Learner.RecordSkillLearning(ctx, evt); err != nil {
		return nil, err
	}
	if err := h.RecallArchiver.ArchiveRecall(ctx, evt); err != nil {
		return nil, err
	}
	return nil, nil
}
