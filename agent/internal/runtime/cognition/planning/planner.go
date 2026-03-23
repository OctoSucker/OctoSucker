package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"strconv"
	"strings"

	"github.com/OctoSucker/agent/internal/runtime/cognition/decision"
	"github.com/OctoSucker/agent/internal/runtime/store"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/mcpclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

type SessionRepository interface {
	Get(id string) (*ports.Session, bool)
	Put(sess *ports.Session) error
}

type RouteGraphWithConfidence interface {
	Confidence(ctx context.Context, rc ports.RoutingContext, last string) float64
}

type PlannerSkillStore interface {
	Match(userText string) []string
	MatchByEmbedding(embedding []float32, k int) []store.SkillEntry
	KeywordPlanEntry(userText string) (store.SkillEntry, bool)
	MarkUsed(name string)
}

type RecallWithRead interface {
	Recall(ctx context.Context, query string, k int) ([]string, error)
}

type Planner struct {
	Router                *decision.Router
	Sessions              SessionRepository
	RouteGraph            RouteGraphWithConfidence
	Skills                PlannerSkillStore
	Embedder              *llmclient.OpenAI
	RecallCorpus          RecallWithRead
	PlannerLLM            *llmclient.OpenAI
	PlanSystemPrompt      string
	ValidPlanCapabilities map[string]ports.Capability
	ToolAppendix          string
	ToolInputSchemas      map[string]any
	SkillRouteThreshold   float64
	GraphRouteThreshold   float64
}

type routeResult struct {
	Decision decision.PolicyDecision
	EmbHit   *store.SkillEntry
	KWEntry  *store.SkillEntry
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
	sess.SkillPriorCaps = nil
	sess.SkillPreferredPath = nil
	sess.ActiveSkillName = ""
	sess.ActiveSkillVariantID = ""
	sess.RouteMode = ""
	sess.RoutePolicy = nil
	sess.RecallContext = ""
	sess.TransitionPath = nil

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
	var embHit *store.SkillEntry
	if len(emb) > 0 {
		hits := p.Skills.MatchByEmbedding(emb, 1)
		if len(hits) > 0 && hits[0].SelectedPlan() != nil {
			h := hits[0]
			embHit = &h
		}
	}

	kwEntry, kwOK := p.Skills.KeywordPlanEntry(pl.Text)
	var kwHit *store.SkillEntry
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
		plan, _, skillPatch = p.selectSkillPlan(route, pl.Text)
		if skillPatch != nil {
			sess.SkillPriorCaps = skillPatch.SkillPriorCaps
			sess.SkillPreferredPath = skillPatch.SkillPreferredPath
			sess.ActiveSkillName = skillPatch.ActiveSkillName
			sess.ActiveSkillVariantID = skillPatch.ActiveSkillVariantID
		}
	} else {
		system := p.PlanSystemPrompt
		if p.ToolAppendix != "" {
			system += "\n\n" + p.ToolAppendix
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

func (p *Planner) route(ctx context.Context, userText string, embHit *store.SkillEntry, kwEntry *store.SkillEntry) routeResult {
	graphConf := p.RouteGraph.Confidence(ctx, ports.RoutingContext{IntentText: userText}, "")
	router := p.Router
	if router == nil {
		router = decision.NewRouter(
			decision.SkillPolicy{EmbeddingThreshold: p.SkillRouteThreshold, KeywordConfidence: 0.92},
			decision.GraphPolicy{Threshold: p.GraphRouteThreshold},
			decision.HeuristicPolicy{},
			decision.PlannerPolicy{},
		)
	}
	routeDecision, ok := router.Decide(ctx, decision.PolicyState{
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

func (p *Planner) selectSkillPlan(r routeResult, userText string) (*ports.Plan, bool, *ports.Session) {
	var plan *ports.Plan
	skillSelected := false
	var patch ports.Session
	if r.Decision.Mode != ports.RouteSkill {
		return nil, false, &patch
	}
	if r.EmbHit != nil && (r.Decision.Reason == "embedding_skill" || r.Decision.Reason == "") {
		plan = r.EmbHit.SelectedPlan()
		patch.SkillPriorCaps = r.EmbHit.Capabilities
		patch.SkillPreferredPath = append([]string(nil), r.EmbHit.Path...)
		patch.ActiveSkillName = r.EmbHit.Name
		patch.ActiveSkillVariantID = r.EmbHit.SelectedVariantID
		p.Skills.MarkUsed(r.EmbHit.Name)
		skillSelected = plan != nil
	} else if r.KWEntry != nil && (r.Decision.Reason == "keyword_skill" || r.Decision.Reason == "") {
		plan = r.KWEntry.SelectedPlan()
		patch.SkillPriorCaps = p.Skills.Match(userText)
		patch.SkillPreferredPath = r.KWEntry.PreferredPath()
		patch.ActiveSkillName = r.KWEntry.Name
		patch.ActiveSkillVariantID = r.KWEntry.SelectedVariantID
		skillSelected = plan != nil
	}
	if plan == nil {
		if r.EmbHit != nil {
			plan = r.EmbHit.SelectedPlan()
			patch.SkillPriorCaps = r.EmbHit.Capabilities
			patch.SkillPreferredPath = append([]string(nil), r.EmbHit.Path...)
			patch.ActiveSkillName = r.EmbHit.Name
			patch.ActiveSkillVariantID = r.EmbHit.SelectedVariantID
			p.Skills.MarkUsed(r.EmbHit.Name)
			skillSelected = plan != nil
		} else if r.KWEntry != nil {
			plan = r.KWEntry.SelectedPlan()
			patch.SkillPriorCaps = p.Skills.Match(userText)
			patch.SkillPreferredPath = r.KWEntry.PreferredPath()
			patch.ActiveSkillName = r.KWEntry.Name
			patch.ActiveSkillVariantID = r.KWEntry.SelectedVariantID
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
