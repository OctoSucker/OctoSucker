package planning

import (
	"fmt"
	"sort"
	"strings"

	"github.com/OctoSucker/agent/internal/runtime/store/nodefailure"
	"github.com/OctoSucker/agent/internal/runtime/store/recall"
	routinggraph "github.com/OctoSucker/agent/internal/runtime/store/routing_graph"
	skill "github.com/OctoSucker/agent/internal/runtime/store/skill"
	"github.com/OctoSucker/agent/internal/runtime/store/task"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

type Planner struct {
	Tasks                 *task.TaskStore
	RouteGraph            *routinggraph.RoutingGraph
	Skills                *skill.SkillRegistry
	SkillRouteThreshold   float64
	GraphRouteThreshold   float64
	KeywordConfidence     float64
	NodeFailures          *nodefailure.NodeFailureStats
	RecallCorpus          *recall.RecallCorpus
	PlannerLLM            *llmclient.OpenAI
	PlanSystemPrompt      string
	ValidPlanCapabilities map[string]ports.Capability
	ToolAppendix          string
	ToolInputSchemas      map[string]any
	// DefaultGraphPathMode: greedy (Frontier) vs global (Dijkstra on feasible candidates); applied each turn on the task.
	DefaultGraphPathMode ports.GraphPathMode
}

// NewPlanner centralizes planner initialization, including system prompt generation.
func NewPlanner(
	skillRouteThreshold float64,
	graphRouteThreshold float64,
	keywordConfidence float64,
	sessions *task.TaskStore,
	routeGraph *routinggraph.RoutingGraph,
	skills *skill.SkillRegistry,
	nodeFailures *nodefailure.NodeFailureStats,
	recallCorpus *recall.RecallCorpus,
	plannerLLM *llmclient.OpenAI,
	validPlanCapabilities map[string]ports.Capability,
	toolAppendix string,
	toolInputSchemas map[string]any,
	defaultGraphPathMode ports.GraphPathMode,
) *Planner {
	planSys := ""
	if len(validPlanCapabilities) > 0 {
		ids := make([]string, 0, len(validPlanCapabilities))
		for id := range validPlanCapabilities {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		exampleCap := ids[0]
		planSys = fmt.Sprintf(`Reply with only one JSON object:
{"steps":[{"id":"1","goal":"string","capability":%q,"depends_on":[],"arguments":{}}]}
Each step's "capability" must be one of: %s.
Include per-step "arguments" matching the MCP tool schema (see appendix in system message). Use {} or omit when no parameters.
Pick the capability that fits the user's request (e.g. questions about the bot or its name: get_telegram_bot_info; sending Telegram text: send_telegram_message).
Use depends_on as array of prior step ids.`, exampleCap, strings.Join(ids, "|"))
	}
	return &Planner{
		Tasks:                 sessions,
		RouteGraph:            routeGraph,
		Skills:                skills,
		SkillRouteThreshold:   skillRouteThreshold,
		GraphRouteThreshold:   graphRouteThreshold,
		KeywordConfidence:     keywordConfidence,
		NodeFailures:          nodeFailures,
		RecallCorpus:          recallCorpus,
		PlannerLLM:            plannerLLM,
		PlanSystemPrompt:      planSys,
		ValidPlanCapabilities: validPlanCapabilities,
		ToolAppendix:          toolAppendix,
		ToolInputSchemas:      toolInputSchemas,
		DefaultGraphPathMode:  defaultGraphPathMode,
	}
}
