package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"sort"
	"strconv"
	"strings"

	"github.com/OctoSucker/agent/internal/runtime/cognition/decision"
	"github.com/OctoSucker/agent/internal/runtime/store/nodefailure"
	"github.com/OctoSucker/agent/internal/runtime/store/recall"
	routinggraph "github.com/OctoSucker/agent/internal/runtime/store/routing_graph"
	"github.com/OctoSucker/agent/internal/runtime/store/session"
	skill "github.com/OctoSucker/agent/internal/runtime/store/skill"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/mcpclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

type Planner struct {
	Router                *decision.Router
	Sessions              *session.SessionStore
	RouteGraph            *routinggraph.RoutingGraph
	Skills                *skill.SkillRegistry
	NodeFailures          *nodefailure.NodeFailureStats
	Embedder              *llmclient.OpenAI
	RecallCorpus          *recall.RecallCorpus
	PlannerLLM            *llmclient.OpenAI
	PlanSystemPrompt      string
	ValidPlanCapabilities map[string]ports.Capability
	ToolAppendix          string
	ToolInputSchemas      map[string]any
	// DefaultGraphPathMode: greedy (Frontier) vs global (Dijkstra toward finish); applied each turn on the session.
	DefaultGraphPathMode ports.GraphPathMode
}

// NewPlanner centralizes planner initialization, including system prompt generation.
func NewPlanner(
	skillRouteThreshold float64,
	graphRouteThreshold float64,
	keywordConfidence float64,
	sessions *session.SessionStore,
	routeGraph *routinggraph.RoutingGraph,
	skills *skill.SkillRegistry,
	nodeFailures *nodefailure.NodeFailureStats,
	embedder *llmclient.OpenAI,
	recallCorpus *recall.RecallCorpus,
	plannerLLM *llmclient.OpenAI,
	validPlanCapabilities map[string]ports.Capability,
	toolAppendix string,
	toolInputSchemas map[string]any,
	defaultGraphPathMode ports.GraphPathMode,
) *Planner {
	return &Planner{
		Router:                decision.NewSpecificRouter(skillRouteThreshold, graphRouteThreshold, keywordConfidence),
		Sessions:              sessions,
		RouteGraph:            routeGraph,
		Skills:                skills,
		NodeFailures:          nodeFailures,
		Embedder:              embedder,
		RecallCorpus:          recallCorpus,
		PlannerLLM:            plannerLLM,
		PlanSystemPrompt:      plannerSystemPrompt(validPlanCapabilities),
		ValidPlanCapabilities: validPlanCapabilities,
		ToolAppendix:          toolAppendix,
		ToolInputSchemas:      toolInputSchemas,
		DefaultGraphPathMode:  defaultGraphPathMode,
	}
}

func (p *Planner) HandleUserInput(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	pl := evt.Payload.(ports.PayloadUserInput)
	sess, ok := p.Sessions.Get(pl.SessionID)
	if !ok {
		sess = &ports.Session{ID: pl.SessionID}
	}
	if !pl.AutoReplan {
		sess.ReplanAllowed = true
		sess.ReplanCount = 0
	}
	sess.UserInput = pl.Text
	sess.Trace = nil
	sess.ToolFailCount = nil
	sess.CapabilityFailCount = nil
	sess.SkillPriorCaps = nil
	sess.SkillPreferredPath = nil
	sess.ActiveSkillName = ""
	sess.ActiveSkillVariantID = ""
	sess.RouteMode = ""
	sess.RoutePolicy = nil
	sess.RecallContext = ""
	sess.TransitionPath = nil
	gpm := p.DefaultGraphPathMode
	sess.GraphPathMode = gpm

	chunks, err := p.RecallCorpus.Recall(ctx, pl.Text, 5)
	if err != nil {
		log.Printf("engine.Dispatcher: recall failed session=%s err=%v", pl.SessionID, err)
		return nil, fmt.Errorf("planner: recall: %w", err)
	}
	if len(chunks) > 0 {
		sess.RecallContext = strings.Join(chunks, "\n---\n")
	}
	sess.SkillPriorCaps = p.Skills.Match(pl.Text)
	user := pl.Text
	if sess.RecallContext != "" {
		user = "相关记忆：\n" + sess.RecallContext + "\n\n用户请求：\n" + pl.Text
	}
	user += telegramChatHint(pl.SessionID)

	emb, err := p.Embedder.Embed(ctx, pl.Text)
	if err != nil {
		log.Printf("engine.Dispatcher: embed failed session=%s err=%v", pl.SessionID, err)
		return nil, fmt.Errorf("planner: embed: %w", err)
	}
	var embHit *skill.SkillEntry
	if len(emb) > 0 {
		hits := p.Skills.MatchByEmbedding(emb, 1)
		if len(hits) > 0 && hits[0].SelectedPlan() != nil {
			h := hits[0]
			embHit = &h
		}
	}

	kwEntry, kwOK := p.Skills.KeywordPlanEntry(pl.Text)
	var kwHit *skill.SkillEntry
	if kwOK && kwEntry.SelectedPlan() != nil {
		e := kwEntry
		kwHit = &e
	}
	route := p.route(ctx, pl.Text, embHit, kwHit)
	mode := route.Decision.Mode
	reason := route.Decision.Reason
	conf := route.Decision.Confidence
	sess.RouteMode = mode
	sess.RoutePolicy = &ports.RoutePolicyDecision{Mode: mode, Confidence: conf, Reason: reason}
	var plan *ports.Plan
	if mode == ports.RouteSkill {
		var skillPatch *ports.Session
		plan, _, skillPatch = p.selectSkillPlan(route, pl.Text, sess)
		if skillPatch != nil {
			sess.SkillPriorCaps = skillPatch.SkillPriorCaps
			sess.SkillPreferredPath = skillPatch.SkillPreferredPath
			sess.ActiveSkillName = skillPatch.ActiveSkillName
			sess.ActiveSkillVariantID = skillPatch.ActiveSkillVariantID
		}
	} else {
		system := p.PlanSystemPrompt
		system += "\n\n" + p.ToolAppendix
		if p.NodeFailures != nil && len(p.ValidPlanCapabilities) > 0 {
			caps := make([]string, 0, len(p.ValidPlanCapabilities))
			for id := range p.ValidPlanCapabilities {
				caps = append(caps, id)
			}
			if hint := p.NodeFailures.PlannerHint(caps); hint != "" {
				system += "\n\n" + hint
			}
		}
		system += "\n\nEach step may include optional \"arguments\": a JSON object used as MCP tools/call arguments for that step. Only keys listed under that tool's params JSON Schema may appear; do not copy the user's message into \"arguments\" unless the schema has a matching field (e.g. echo.text, send_telegram_message.text). Tools whose schema has no properties must use {} or omit \"arguments\". If one capability runs multiple tools in sequence, the same arguments object is sent to each—use separate steps when schemas differ."
		raw, err := p.PlannerLLM.Complete(ctx, system, user)
		if err != nil {
			log.Printf("engine.Dispatcher: LLM Complete failed session=%s err=%v", pl.SessionID, err)
			return nil, fmt.Errorf("planner: llm: %w", err)
		}
		parsed := parsePlanJSON(raw, p.ValidPlanCapabilities)
		if parsed == nil {
			log.Printf("engine.Dispatcher: invalid plan JSON session=%s raw_len=%d", pl.SessionID, len(raw))
			return nil, fmt.Errorf("planner: llm returned invalid or empty plan json")
		}
		plan = parsed
	}
	if plan == nil || len(plan.Steps) == 0 {
		return nil, fmt.Errorf("planner: empty plan")
	}
	if err := validatePlanToolArguments(plan, p.ToolInputSchemas); err != nil {
		log.Printf("engine.Dispatcher: plan arguments invalid session=%s err=%v", pl.SessionID, err)
		return nil, fmt.Errorf("planner: plan tool arguments: %w", err)
	}
	for i := range plan.Steps {
		if plan.Steps[i].Status == "" {
			plan.Steps[i].Status = "pending"
		}
	}
	sess.Plan = plan
	sess.LastCapability = ""
	sess.LastOutcome = 0
	if err := p.Sessions.Put(sess); err != nil {
		return nil, err
	}
	return []ports.Event{{Type: ports.EvPlanCreated, Payload: ports.PayloadPlanCreated{SessionID: pl.SessionID}}}, nil
}

func (p *Planner) route(ctx context.Context, userText string, embHit *skill.SkillEntry, kwEntry *skill.SkillEntry) routeResult {
	graphConf := p.RouteGraph.Confidence(ctx, ports.RoutingContext{IntentText: userText}, "")
	routeDecision, ok := p.Router.Decide(ctx, decision.PolicyState{
		UserText:        userText,
		GraphConfidence: graphConf,
		EmbeddingHit:    embHit,
		KeywordHit:      kwEntry,
	})
	if !ok {
		routeDecision = decision.PolicyDecision{Mode: ports.RoutePlanner, Confidence: 0, Reason: "planner"}
	}
	return routeResult{Decision: routeDecision, EmbHit: embHit, KWEntry: kwEntry}
}

func (p *Planner) selectSkillPlan(r routeResult, userText string, sess *ports.Session) (*ports.Plan, bool, *ports.Session) {
	var plan *ports.Plan
	skillSelected := false
	var patch ports.Session
	if r.Decision.Mode != ports.RouteSkill {
		return nil, false, &patch
	}
	buildPlan := func(e *skill.SkillEntry) *ports.Plan {
		if e == nil {
			return nil
		}
		v := e.SelectedVariant()
		if v == nil || v.Plan == nil {
			return nil
		}
		inv := skill.InvocationContext{UserInput: userText}
		if sess != nil {
			inv.Trace = sess.Trace
		}
		pl, err := skill.InvokeSkillVariant(v, inv)
		if err != nil {
			log.Printf("planner: skill invocation failed skill=%s variant=%s err=%v", e.Name, v.ID, err)
			return nil
		}
		return pl
	}
	if r.EmbHit != nil && (r.Decision.Reason == "embedding_skill" || r.Decision.Reason == "") {
		plan = buildPlan(r.EmbHit)
		patch.SkillPriorCaps = r.EmbHit.Capabilities
		patch.SkillPreferredPath = append([]string(nil), r.EmbHit.Path...)
		patch.ActiveSkillName = r.EmbHit.Name
		patch.ActiveSkillVariantID = r.EmbHit.SelectedVariantID
		if plan != nil {
			p.Skills.MarkUsed(r.EmbHit.Name, r.EmbHit.SelectedVariantID)
		}
		skillSelected = plan != nil
	} else if r.KWEntry != nil && (r.Decision.Reason == "keyword_skill" || r.Decision.Reason == "") {
		plan = buildPlan(r.KWEntry)
		patch.SkillPriorCaps = p.Skills.Match(userText)
		patch.SkillPreferredPath = r.KWEntry.PreferredPath()
		patch.ActiveSkillName = r.KWEntry.Name
		patch.ActiveSkillVariantID = r.KWEntry.SelectedVariantID
		if plan != nil {
			p.Skills.MarkUsed(r.KWEntry.Name, r.KWEntry.SelectedVariantID)
		}
		skillSelected = plan != nil
	}
	if plan == nil {
		if r.EmbHit != nil {
			plan = buildPlan(r.EmbHit)
			patch.SkillPriorCaps = r.EmbHit.Capabilities
			patch.SkillPreferredPath = append([]string(nil), r.EmbHit.Path...)
			patch.ActiveSkillName = r.EmbHit.Name
			patch.ActiveSkillVariantID = r.EmbHit.SelectedVariantID
			if plan != nil {
				p.Skills.MarkUsed(r.EmbHit.Name, r.EmbHit.SelectedVariantID)
			}
			skillSelected = plan != nil
		} else if r.KWEntry != nil {
			plan = buildPlan(r.KWEntry)
			patch.SkillPriorCaps = p.Skills.Match(userText)
			patch.SkillPreferredPath = r.KWEntry.PreferredPath()
			patch.ActiveSkillName = r.KWEntry.Name
			patch.ActiveSkillVariantID = r.KWEntry.SelectedVariantID
			if plan != nil {
				p.Skills.MarkUsed(r.KWEntry.Name, r.KWEntry.SelectedVariantID)
			}
			skillSelected = plan != nil
		}
	}
	return plan, skillSelected, &patch
}

func telegramChatHint(sessionID string) string {
	if strings.HasPrefix(sessionID, "http-") {
		return ""
	}
	id, err := strconv.ParseInt(sessionID, 10, 64)
	if err != nil {
		return ""
	}
	if strconv.FormatInt(id, 10) != sessionID {
		return ""
	}
	return fmt.Sprintf("\n\n[Channel: Telegram; current chat_id is %d. Include it as \"chat_id\" in step arguments when a tool requires chat_id.]", id)
}

func parsePlanJSON(s string, allow map[string]ports.Capability) *ports.Plan {
	if len(allow) == 0 {
		return nil
	}
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	var x struct {
		Steps []struct {
			ID         string         `json:"id"`
			Goal       string         `json:"goal"`
			Capability string         `json:"capability"`
			DependsOn  []string       `json:"depends_on"`
			Arguments  map[string]any `json:"arguments"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(s), &x); err != nil || len(x.Steps) == 0 {
		return nil
	}
	out := &ports.Plan{}
	for _, st := range x.Steps {
		if st.ID == "" || st.Capability == "" {
			return nil
		}
		if _, ok := allow[st.Capability]; !ok {
			return nil
		}
		out.Steps = append(out.Steps, ports.PlanStep{
			ID:         st.ID,
			Goal:       st.Goal,
			Capability: st.Capability,
			DependsOn:  st.DependsOn,
			Arguments:  maps.Clone(st.Arguments),
			Status:     "pending",
		})
	}
	return out
}

func validatePlanToolArguments(plan *ports.Plan, schemaByTool map[string]any) error {
	if plan == nil || len(schemaByTool) == 0 {
		return nil
	}
	for _, st := range plan.Steps {
		schema, ok := schemaByTool[st.Capability]
		if !ok {
			return fmt.Errorf("no input schema for capability %q", st.Capability)
		}
		if err := mcpclient.ValidateToolArguments(st.Capability, st.Arguments, schema); err != nil {
			return fmt.Errorf("step id=%q capability=%q: %w", st.ID, st.Capability, err)
		}
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

type routeResult struct {
	Decision decision.PolicyDecision
	EmbHit   *skill.SkillEntry
	KWEntry  *skill.SkillEntry
}
