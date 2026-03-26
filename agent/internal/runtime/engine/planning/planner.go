package planning

import (
	"fmt"
	"sort"
	"strings"

	"github.com/OctoSucker/agent/internal/runtime/store/capability"
	"github.com/OctoSucker/agent/internal/runtime/store/nodefailure"
	procedure "github.com/OctoSucker/agent/internal/runtime/store/procedure"
	"github.com/OctoSucker/agent/internal/runtime/store/recall"
	routinggraph "github.com/OctoSucker/agent/internal/runtime/store/routing_graph"
	"github.com/OctoSucker/agent/internal/runtime/store/task"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

type Planner struct {
	Tasks            *task.TaskStore
	RouteGraph       *routinggraph.RoutingGraph
	Procedures       *procedure.ProcedureRegistry
	NodeFailures     *nodefailure.NodeFailureStats
	RecallCorpus     *recall.RecallCorpus
	PlannerLLM       *llmclient.OpenAI
	PlanSystemPrompt string
	CapRegistry      *capability.CapabilityRegistry
	ToolAppendix     string
	// DefaultGraphPathMode: greedy (Frontier) vs global (Dijkstra on feasible candidates); applied each turn on the task.
	DefaultGraphPathMode ports.GraphPathMode
}

// NewPlanner centralizes planner initialization, including system prompt generation.
func NewPlanner(
	tasks *task.TaskStore,
	routeGraph *routinggraph.RoutingGraph,
	procedures *procedure.ProcedureRegistry,
	nodeFailures *nodefailure.NodeFailureStats,
	recallCorpus *recall.RecallCorpus,
	plannerLLM *llmclient.OpenAI,
	capReg *capability.CapabilityRegistry,
	toolAppendix string,
) *Planner {

	planSys := ""
	if capReg != nil {
		valid := capReg.AllCapabilities()
		if len(valid) > 0 {
			ids := make([]string, 0, len(valid))
			for id := range valid {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			exampleCap := ids[0]
			planSys = fmt.Sprintf(`Reply with only one JSON object:
{"steps":[{"id":"1","goal":"string","capability":%q,"tool":"exact_mcp_tool_name_if_needed","depends_on":[],"arguments":{}}]}
Each step's "capability" must be one of: %s.
When the appendix lists more than one tool under that capability, set "tool" to the exact MCP tool name (e.g. send_telegram_message). Omit "tool" only when that capability has exactly one tool.
Include per-step "arguments" matching that tool's input schema (see appendix). Use {} or omit when no parameters.
Pick the capability that fits the user's request (e.g. listing files: exec server tools; Telegram replies: telegram tools with the right "tool" and schema).
Use depends_on as array of prior step ids.`, exampleCap, strings.Join(ids, "|"))
		}
	}
	return &Planner{
		Tasks:                tasks,
		RouteGraph:           routeGraph,
		Procedures:           procedures,
		NodeFailures:         nodeFailures,
		RecallCorpus:         recallCorpus,
		PlannerLLM:           plannerLLM,
		PlanSystemPrompt:     planSys,
		CapRegistry:          capReg,
		ToolAppendix:         toolAppendix,
		DefaultGraphPathMode: ports.GraphPathGreedy,
	}
}
