package engine

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/internal/runtime/store"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/mcpclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

type Dispatcher struct {
	MaxSteps              int
	Sessions              *store.SessionStore
	RouteGraph            *store.RoutingGraph
	CapRegistry           *store.CapabilityRegistry
	MCPRouter             *mcpclient.MCPRouter
	Skills                *store.SkillRegistry
	Embedder              *llmclient.OpenAI
	RecallCorpus          *store.RecallCorpus
	PlannerLLM            *llmclient.OpenAI
	TrajectoryLLM         *llmclient.OpenAI
	PlanSystemPrompt      string
	ValidPlanCapabilities map[string]ports.Capability
	ToolAppendix          string
	ToolInputSchemas      map[string]any
	SkillRouteThreshold   float64
	GraphRouteThreshold   float64
	MaxFailsPerTool       int
	ExtractScoreThreshold float64
}

func NewDispatcher(
	maxSteps int,
	mcpRouter *mcpclient.MCPRouter,
	capabilities map[string]ports.Capability,
	openai config.OpenAI,
) *Dispatcher {

	sessions := store.NewSessionStore()
	routeGraph := store.NewRoutingGraphFromCapabilities(capabilities)
	skills := store.NewSkillRegistry()
	o := openai
	plannerLLM := llmclient.NewOpenAI(o.BaseURL, o.APIKey, o.Model, o.EmbeddingModel)
	embedder := llmclient.NewOpenAI(o.BaseURL, o.APIKey, o.Model, o.EmbeddingModel)
	trajectoryLLM := llmclient.NewOpenAI(o.BaseURL, o.APIKey, o.Model, o.EmbeddingModel)
	recallCorpus := store.NewRecallCorpus(embedder)
	capReg := store.NewCapabilityRegistryFromCapabilities(capabilities)

	toolApp := ""
	schemaByTool := map[string]any{}
	if mcpRouter != nil {
		for _, spec := range mcpRouter.CachedToolSpecs() {
			schemaByTool[spec.Name] = spec.InputSchema
		}
		toolApp = mcpclient.PlannerToolAppendix(mcpRouter.CachedToolSpecs())
	}
	return &Dispatcher{
		MaxSteps:              maxSteps,
		Sessions:              sessions,
		RouteGraph:            routeGraph,
		CapRegistry:           capReg,
		MCPRouter:             mcpRouter,
		Skills:                skills,
		Embedder:              embedder,
		RecallCorpus:          recallCorpus,
		PlannerLLM:            plannerLLM,
		TrajectoryLLM:         trajectoryLLM,
		PlanSystemPrompt:      plannerSystemPrompt(capabilities),
		ValidPlanCapabilities: capabilities,
		ToolAppendix:          toolApp,
		ToolInputSchemas:      schemaByTool,
		SkillRouteThreshold:   0.9,
		GraphRouteThreshold:   0.7,
		MaxFailsPerTool:       2,
		ExtractScoreThreshold: 0.8,
	}
}

func (d *Dispatcher) dispatchEvent(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	if d == nil {
		return nil, nil
	}
	switch evt.Type {
	case ports.EvUserInput:
		return d.plnnerUserInput(ctx, evt)
	case ports.EvPlanCreated, ports.EvStepCompleted, ports.EvStepCapabilityRetry:
		return d.planExecutorPlanProgress(ctx, evt)
	case ports.EvToolCall:
		return d.toolExecutorToolCall(ctx, evt)
	case ports.EvObservationReady:
		return d.stepCriticObservationReady(ctx, evt)
	case ports.EvTrajectoryCheck:
		return d.trajectoryCriticTrajectoryCheck(ctx, evt)
	case ports.EvTurnFinalized:
		if err := d.recordSkillLearning(ctx, evt); err != nil {
			return nil, err
		}
		if err := d.archiveRecall(ctx, evt); err != nil {
			return nil, err
		}
		return nil, nil
	default:
		return nil, nil
	}
}

func (d *Dispatcher) Run(ctx context.Context, queue []ports.Event) error {
	if d == nil {
		return fmt.Errorf("engine: nil dispatcher")
	}
	if d.MaxSteps <= 0 {
		return fmt.Errorf("engine: MaxSteps must be > 0")
	}
	n := 0
	for len(queue) > 0 && n < d.MaxSteps {
		evt := queue[0]
		queue = queue[1:]
		n++
		out, err := d.dispatchEvent(ctx, evt)
		if err != nil {
			log.Printf("engine.Dispatcher.Run: abort event=%s iter=%d err=%v", evt.Type, n, err)
			return err
		}
		queue = append(queue, out...)
	}
	return nil
}

func plannerSystemPrompt(m map[string]ports.Capability) string {
	if len(m) == 0 {
		return ""
	}
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	exampleCap := ids[0]
	for _, id := range ids {
		if id != "echo" {
			exampleCap = id
			break
		}
	}
	return fmt.Sprintf(`Reply with only one JSON object:
{"steps":[{"id":"1","goal":"string","capability":%q,"depends_on":[],"arguments":{}}]}
Each step's "capability" must be one of: %s.
Include per-step "arguments" matching the MCP tool schema (see appendix in system message). Use {} or omit when no parameters.
Pick the capability that fits the user's request (e.g. questions about the bot or its name: get_telegram_bot_info; sending Telegram text: send_telegram_message; do not use echo unless the user explicitly wants their text repeated).
Use depends_on as array of prior step ids.`, exampleCap, strings.Join(ids, "|"))
}
