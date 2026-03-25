package planning

import (
	"context"
	"fmt"
	"log"

	skill "github.com/OctoSucker/agent/internal/runtime/store/skill"
	"github.com/OctoSucker/agent/pkg/ports"
)

func (p *Planner) HandleSkillPlanRequested(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadSkillPlanRequested)
	sess, ok := p.Tasks.Get(pl.TaskID)
	if !ok {
		return nil, fmt.Errorf("planner: task %q not found", pl.TaskID)
	}
	var plan *ports.Plan
	var patch ports.Task
	routeType := sess.RoutePolicy.Type
	switch routeType {
	case ports.RouteTypeEmbeddingSkill:
		embHit, err := p.Skills.MatchBestByText(ctx, sess.UserInput.Text)
		if err != nil {
			return nil, err
		}
		if embHit == nil {
			return nil, fmt.Errorf("planner: routeType=%q but embedding skill hit is nil", routeType)
		}
		plan = p.buildSkillPlan(embHit, sess)
		patch.SkillPriorCaps = embHit.Capabilities
		patch.SkillPreferredPath = append([]string(nil), embHit.Path...)
		patch.ActiveSkillName = embHit.Name
		patch.ActiveSkillVariantID = embHit.SelectedVariantID
		if plan != nil {
			if err := p.Skills.MarkUsed(embHit.Name, embHit.SelectedVariantID); err != nil {
				return nil, fmt.Errorf("planner: mark skill used: %w", err)
			}
		}
	case ports.RouteTypeKeywordSkill:
		kwHit := p.Skills.KeywordPlanHit(sess.UserInput.Text)
		if kwHit == nil {
			return nil, fmt.Errorf("planner: routeType=%q but keyword skill hit is nil", routeType)
		}
		plan = p.buildSkillPlan(kwHit, sess)
		patch.SkillPriorCaps = p.Skills.Match(sess.UserInput.Text)
		patch.SkillPreferredPath = kwHit.PreferredPath()
		patch.ActiveSkillName = kwHit.Name
		patch.ActiveSkillVariantID = kwHit.SelectedVariantID
		if plan != nil {
			if err := p.Skills.MarkUsed(kwHit.Name, kwHit.SelectedVariantID); err != nil {
				return nil, fmt.Errorf("planner: mark skill used: %w", err)
			}
		}
	default:
		return nil, fmt.Errorf("planner: unexpected route type for skill plan: %q", routeType)
	}
	sess.SkillPriorCaps = patch.SkillPriorCaps
	sess.SkillPreferredPath = patch.SkillPreferredPath
	sess.ActiveSkillName = patch.ActiveSkillName
	sess.ActiveSkillVariantID = patch.ActiveSkillVariantID
	return p.finalizePlan(pl.TaskID, sess, plan)
}

func (p *Planner) buildSkillPlan(e *skill.SkillEntry, sess *ports.Task) *ports.Plan {
	if e == nil {
		return nil
	}
	v := e.SelectedVariant()
	if v == nil || v.Plan == nil {
		return nil
	}
	inv := skill.InvocationContext{UserInput: sess.UserInput.Text, Trace: sess.Trace}
	pln, err := skill.InvokeSkillVariant(v, inv)
	if err != nil {
		log.Printf("planner: skill invocation failed skill=%s variant=%s err=%v", e.Name, v.ID, err)
		return nil
	}
	return pln
}
