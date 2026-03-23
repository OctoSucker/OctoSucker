package engine

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/internal/runtime/cognition/decision"
	"github.com/OctoSucker/agent/internal/runtime/cognition/evaluation"
	"github.com/OctoSucker/agent/internal/runtime/cognition/learning"
	"github.com/OctoSucker/agent/internal/runtime/cognition/memory"
	"github.com/OctoSucker/agent/internal/runtime/cognition/planning"
	"github.com/OctoSucker/agent/internal/runtime/execution"
	"github.com/OctoSucker/agent/internal/runtime/store"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/mcpclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

type EventHandler func(context.Context, ports.Event) ([]ports.Event, error)

// AgentRuntime owns only the event-loop scheduling mechanics.
type AgentRuntime struct {
	MaxSteps int
}

func (r *AgentRuntime) Run(ctx context.Context, queue []ports.Event, dispatch EventHandler) error {
	if r == nil {
		return fmt.Errorf("engine: nil runtime")
	}
	if dispatch == nil {
		return fmt.Errorf("engine: nil dispatch handler")
	}
	if r.MaxSteps <= 0 {
		return fmt.Errorf("engine: MaxSteps must be > 0")
	}
	n := 0
	for len(queue) > 0 && n < r.MaxSteps {
		evt := queue[0]
		queue = queue[1:]
		n++
		out, err := dispatch(ctx, evt)
		if err != nil {
			log.Printf("engine.AgentRuntime.Run: abort event=%s iter=%d err=%v", evt.Type, n, err)
			return err
		}
		queue = append(queue, out...)
	}
	return nil
}

// AgentBrain collects cognitive dependencies (planning/critic/learning/memory access).
type AgentBrain struct {
	Router                *decision.Router
	Planner               *planning.Planner
	StepCritic            *evaluation.StepCritic
	TrajectoryCritic      *evaluation.TrajectoryCritic
	Learner               *learning.Learner
	RecallArchiver        *memory.RecallArchiver
	Sessions              SessionRepository
	RouteGraph            RouteGraphStore
	Skills                SkillStore
	Embedder              *llmclient.OpenAI
	RecallCorpus          RecallStore
	PlannerLLM            *llmclient.OpenAI
	TrajectoryLLM         *llmclient.OpenAI
	PlanSystemPrompt      string
	ValidPlanCapabilities map[string]ports.Capability
	ToolAppendix          string
	ToolInputSchemas      map[string]any
	SkillRouteThreshold   float64
	GraphRouteThreshold   float64
	ExtractScoreThreshold float64
}

// AgentExecutor collects dependencies needed for plan/tool execution decisions.
type AgentExecutor struct {
	ToolExec        *execution.ToolExecutor
	PlanExec        *execution.PlanExecutor
	Sessions        SessionRepository
	RouteGraph      RouteGraphStore
	CapRegistry     CapabilityStore
	MCPRouter       ToolInvoker
	MaxFailsPerTool int
}

type Dispatcher struct {
	Runtime               *AgentRuntime
	Brain                 *AgentBrain
	Executor              *AgentExecutor
	handlers              map[string]EventHandler
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
	d := &Dispatcher{
		Runtime:               &AgentRuntime{MaxSteps: maxSteps},
		handlers:              map[string]EventHandler{},
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
	d.Brain = &AgentBrain{
		Router: decision.NewRouter(
			decision.SkillPolicy{EmbeddingThreshold: d.SkillRouteThreshold, KeywordConfidence: 0.92},
			decision.GraphPolicy{Threshold: d.GraphRouteThreshold},
			decision.HeuristicPolicy{},
			decision.PlannerPolicy{},
		),
		Planner: &planning.Planner{
			Router:                decision.NewRouter(decision.SkillPolicy{EmbeddingThreshold: d.SkillRouteThreshold, KeywordConfidence: 0.92}, decision.GraphPolicy{Threshold: d.GraphRouteThreshold}, decision.HeuristicPolicy{}, decision.PlannerPolicy{}),
			Sessions:              d.Sessions,
			RouteGraph:            d.RouteGraph,
			Skills:                d.Skills,
			Embedder:              d.Embedder,
			RecallCorpus:          d.RecallCorpus,
			PlannerLLM:            d.PlannerLLM,
			PlanSystemPrompt:      d.PlanSystemPrompt,
			ValidPlanCapabilities: d.ValidPlanCapabilities,
			ToolAppendix:          d.ToolAppendix,
			ToolInputSchemas:      d.ToolInputSchemas,
			SkillRouteThreshold:   d.SkillRouteThreshold,
			GraphRouteThreshold:   d.GraphRouteThreshold,
		},
		StepCritic: &evaluation.StepCritic{
			Sessions:        d.Sessions,
			RouteGraph:      d.RouteGraph,
			CapRegistry:     d.CapRegistry,
			MaxFailsPerTool: d.MaxFailsPerTool,
		},
		TrajectoryCritic: &evaluation.TrajectoryCritic{
			Sessions:      d.Sessions,
			TrajectoryLLM: d.TrajectoryLLM,
		},
		Learner: &learning.Learner{
			Sessions:              d.Sessions,
			Skills:                d.Skills,
			RouteGraph:            d.RouteGraph,
			Embedder:              d.Embedder,
			SkillRouteThreshold:   d.SkillRouteThreshold,
			ExtractScoreThreshold: d.ExtractScoreThreshold,
		},
		RecallArchiver: &memory.RecallArchiver{
			Sessions: d.Sessions,
			Recall:   d.RecallCorpus,
		},
		Sessions:              d.Sessions,
		RouteGraph:            d.RouteGraph,
		Skills:                d.Skills,
		Embedder:              d.Embedder,
		RecallCorpus:          d.RecallCorpus,
		PlannerLLM:            d.PlannerLLM,
		TrajectoryLLM:         d.TrajectoryLLM,
		PlanSystemPrompt:      d.PlanSystemPrompt,
		ValidPlanCapabilities: d.ValidPlanCapabilities,
		ToolAppendix:          d.ToolAppendix,
		ToolInputSchemas:      d.ToolInputSchemas,
		SkillRouteThreshold:   d.SkillRouteThreshold,
		GraphRouteThreshold:   d.GraphRouteThreshold,
		ExtractScoreThreshold: d.ExtractScoreThreshold,
	}
	d.Executor = &AgentExecutor{
		ToolExec: &execution.ToolExecutor{
			Sessions: d.Sessions,
			Invoker:  d.MCPRouter,
		},
		PlanExec: &execution.PlanExecutor{
			Sessions:    d.Sessions,
			RouteGraph:  d.RouteGraph,
			CapRegistry: d.CapRegistry,
			MCPRouter:   d.MCPRouter,
		},
		Sessions:        d.Sessions,
		RouteGraph:      d.RouteGraph,
		CapRegistry:     d.CapRegistry,
		MCPRouter:       d.MCPRouter,
		MaxFailsPerTool: d.MaxFailsPerTool,
	}
	d.registerDefaultHandlers()
	return d
}

func (d *Dispatcher) registerDefaultHandlers() {
	if d == nil {
		return
	}
	d.handlers = map[string]EventHandler{
		ports.EvUserInput:           d.Brain.Planner.HandleUserInput,
		ports.EvPlanCreated:         d.Executor.PlanExec.HandlePlanProgress,
		ports.EvStepCompleted:       d.Executor.PlanExec.HandlePlanProgress,
		ports.EvStepCapabilityRetry: d.Executor.PlanExec.HandlePlanProgress,
		ports.EvToolCall:            d.Executor.ToolExec.HandleToolCall,
		ports.EvObservationReady:    d.Brain.StepCritic.HandleObservationReady,
		ports.EvTrajectoryCheck:     d.Brain.TrajectoryCritic.HandleTrajectoryCheck,
		ports.EvTurnFinalized:       d.turnFinalized,
	}
}

func (d *Dispatcher) turnFinalized(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	if err := d.Brain.Learner.RecordSkillLearning(ctx, evt); err != nil {
		return nil, err
	}
	if err := d.Brain.RecallArchiver.ArchiveRecall(ctx, evt); err != nil {
		return nil, err
	}
	return nil, nil
}

func (d *Dispatcher) dispatchEvent(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	if d == nil {
		return nil, nil
	}
	h, ok := d.handlers[evt.Type]
	if !ok {
		return nil, nil
	}
	return h(ctx, evt)
}

func (d *Dispatcher) Run(ctx context.Context, queue []ports.Event) error {
	if d == nil {
		return fmt.Errorf("engine: nil dispatcher")
	}
	if d.Runtime == nil {
		d.Runtime = &AgentRuntime{MaxSteps: d.MaxSteps}
	}
	if len(d.handlers) == 0 {
		d.registerDefaultHandlers()
	}
	return d.Runtime.Run(ctx, queue, d.dispatchEvent)
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
