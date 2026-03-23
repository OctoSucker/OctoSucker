package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"strconv"
	"strings"

	"github.com/OctoSucker/agent/internal/runtime/store"
	"github.com/OctoSucker/agent/pkg/mcpclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

func (d *Dispatcher) plnnerUserInput(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	pl := evt.Payload.(ports.PayloadUserInput)
	sess, ok := d.Sessions.Get(pl.SessionID)
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
	sess.RouteMode = ""
	sess.RoutePolicy = nil
	sess.RecallContext = ""
	sess.TransitionPath = nil

	chunks, err := d.RecallCorpus.Recall(ctx, pl.Text, 5)
	if err != nil {
		log.Printf("engine.Dispatcher: recall failed session=%s err=%v", pl.SessionID, err)
		return nil, fmt.Errorf("planner: recall: %w", err)
	}
	if len(chunks) > 0 {
		sess.RecallContext = strings.Join(chunks, "\n---\n")
	}
	sess.SkillPriorCaps = d.Skills.Match(pl.Text)
	user := pl.Text
	if sess.RecallContext != "" {
		user = "相关记忆：\n" + sess.RecallContext + "\n\n用户请求：\n" + pl.Text
	}
	user += telegramChatHint(pl.SessionID)

	graphConf := d.RouteGraph.Confidence(ctx, ports.RoutingContext{IntentText: pl.Text}, "")
	emb, err := d.Embedder.Embed(ctx, pl.Text)
	if err != nil {
		log.Printf("engine.Dispatcher: embed failed session=%s err=%v", pl.SessionID, err)
		return nil, fmt.Errorf("planner: embed: %w", err)
	}
	var embHit *store.SkillEntry
	if len(emb) > 0 {
		hits := d.Skills.MatchByEmbedding(emb, 1)
		if len(hits) > 0 && hits[0].Plan != nil {
			h := hits[0]
			embHit = &h
		}
	}

	kwEntry, kwOK := d.Skills.KeywordPlanEntry(pl.Text)
	mode := ports.RoutePlanner
	reason := "fallback"
	conf := graphConf
	switch {
	case embHit != nil && embHit.MatchScore >= d.SkillRouteThreshold:
		mode = ports.RouteSkill
		reason = "embedding_skill"
		conf = embHit.MatchScore
	case graphConf >= d.GraphRouteThreshold:
		mode = ports.RouteGraph
		reason = "graph_confidence"
		conf = graphConf
	case kwOK && kwEntry.Plan != nil:
		mode = ports.RouteSkill
		reason = "keyword_skill"
		conf = 0.92
	default:
		mode = ports.RoutePlanner
		reason = "planner"
		conf = 0
	}
	sess.RouteMode = mode
	sess.RoutePolicy = &ports.RoutePolicyDecision{Mode: mode, Confidence: conf, Reason: reason}
	var plan *ports.Plan
	if mode == ports.RouteSkill {
		if embHit != nil && embHit.MatchScore >= d.SkillRouteThreshold {
			plan = embHit.Plan
			sess.SkillPriorCaps = embHit.Capabilities
			sess.SkillPreferredPath = append([]string(nil), embHit.Path...)
			d.Skills.MarkUsed(embHit.Name)
		} else if kwOK && kwEntry.Plan != nil {
			plan = store.CloneSkillPlan(kwEntry.Plan)
			sess.SkillPriorCaps = d.Skills.Match(pl.Text)
			sess.SkillPreferredPath = kwEntry.PreferredPath()
		}
	} else {
		system := d.PlanSystemPrompt
		if d.ToolAppendix != "" {
			system += "\n\n" + d.ToolAppendix
		}
		system += "\n\nEach step may include optional \"arguments\": a JSON object used as MCP tools/call arguments for that step. Only keys listed under that tool's params JSON Schema may appear; do not copy the user's message into \"arguments\" unless the schema has a matching field (e.g. echo.text, send_telegram_message.text). Tools whose schema has no properties must use {} or omit \"arguments\". If one capability runs multiple tools in sequence, the same arguments object is sent to each—use separate steps when schemas differ."
		raw, err := d.PlannerLLM.Complete(ctx, system, user)
		if err != nil {
			log.Printf("engine.Dispatcher: LLM Complete failed session=%s err=%v", pl.SessionID, err)
			return nil, fmt.Errorf("planner: llm: %w", err)
		}
		parsed := parsePlanJSON(raw, d.ValidPlanCapabilities)
		if parsed == nil {
			log.Printf("engine.Dispatcher: invalid plan JSON session=%s raw_len=%d", pl.SessionID, len(raw))
			return nil, fmt.Errorf("planner: llm returned invalid or empty plan json")
		}
		plan = parsed
	}
	if plan == nil || len(plan.Steps) == 0 {
		return nil, fmt.Errorf("planner: empty plan")
	}
	if err := validatePlanToolArguments(plan, d.ToolInputSchemas); err != nil {
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
	if err := d.Sessions.Put(sess); err != nil {
		return nil, err
	}
	return []ports.Event{{Type: ports.EvPlanCreated, Payload: ports.PayloadPlanCreated{SessionID: pl.SessionID}}}, nil
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
