package planning

import (
	"fmt"
	"sort"
	"strings"

	"github.com/OctoSucker/agent/internal/store/capability"
	skillsbuiltin "github.com/OctoSucker/agent/internal/store/capability/builtin/skills"
	"github.com/OctoSucker/agent/internal/store/nodefailure"
	procedure "github.com/OctoSucker/agent/internal/store/procedure"
	"github.com/OctoSucker/agent/internal/store/recall"
	routinggraph "github.com/OctoSucker/agent/internal/store/routing_graph"
	"github.com/OctoSucker/agent/internal/store/task"
	"github.com/OctoSucker/agent/pkg/llmclient"
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
	SkillStore       *skillsbuiltin.Store
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
	skillStore *skillsbuiltin.Store,
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
When the appendix lists more than one tool under that capability, set "tool" to the exact tool name. Omit "tool" only when that capability has exactly one tool. For exec, the appendix uses a single open-ended entry: always set "tool" to the argv0 to run (any non-empty name).
Include per-step "arguments" matching that tool's input schema (see appendix). Use {} or omit when no parameters.
Pick the capability that fits the user's request (e.g. shell operations: exec capability).
For exec capability, set "tool" to the program name (e.g. ls, cat, git, sh). Arguments: optional "args" (string array, argv after the program name), optional work_dir, timeout_sec, env. For shell one-liners use tool "sh" or "bash" with args like ["-c", "your script"].
Use depends_on as array of prior step ids.`, exampleCap, strings.Join(ids, "|"))
		}
	}
	return &Planner{
		Tasks:            tasks,
		RouteGraph:       routeGraph,
		Procedures:       procedures,
		NodeFailures:     nodeFailures,
		RecallCorpus:     recallCorpus,
		PlannerLLM:       plannerLLM,
		PlanSystemPrompt: planSys,
		CapRegistry:      capReg,
		SkillStore:       skillStore,
	}
}
