package store

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"sync"
	"time"

	rtutils "github.com/OctoSucker/agent/utils"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

// SkillPlanVariant is one executable plan bound to the same capability sequence (macro skill).
// Multiple variants allow skill evolution without overwriting a previously learned plan.
type SkillPlanVariant struct {
	ID        string
	Plan      *ports.Plan
	Attempts  int
	Successes int
}

// SkillEntry groups a trigger, a capability procedure, and zero or more plan variants.
type SkillEntry struct {
	Name         string
	Keywords     []string
	Capabilities []string
	Path         []string
	// TriggerEmbedding is matched against the user query embedding (learned / optional static).
	TriggerEmbedding []float32
	Variants         []SkillPlanVariant
	// Attempts/Successes are skill-level counters (keyword hits, implicit embedding reinforcement,
	// and attributed turns). Variant-level stats live on SkillPlanVariant.
	Attempts   int
	Successes  int
	LastUsedAt time.Time
	MatchScore float64
	// SelectedVariantID is set only on routing snapshots returned to the planner (not stored in registry).
	SelectedVariantID string `json:"-"`
}

func (e SkillEntry) SuccessRate() float64 {
	if e.Attempts <= 0 {
		return 1
	}
	return float64(e.Successes) / float64(e.Attempts)
}

func (e SkillEntry) tooPoorForMatch() bool {
	return e.Attempts > 5 && e.SuccessRate() < 0.3
}

// SelectedPlan returns a deep copy of the plan for the selected or best-scoring variant.
func (e SkillEntry) SelectedPlan() *ports.Plan {
	if len(e.Variants) == 0 {
		return nil
	}
	if e.SelectedVariantID != "" {
		for i := range e.Variants {
			if e.Variants[i].ID == e.SelectedVariantID {
				return CloneSkillPlan(e.Variants[i].Plan)
			}
		}
	}
	bi := bestVariantIndex(e.Variants)
	if bi < 0 {
		return nil
	}
	return CloneSkillPlan(e.Variants[bi].Plan)
}

func (e SkillEntry) PreferredPath() []string {
	return pathOrCaps(e)
}

type SkillRegistry struct {
	mu      sync.RWMutex
	entries []SkillEntry
	db      *sql.DB
}

func NewSkillRegistry(db *sql.DB) *SkillRegistry {
	r := &SkillRegistry{entries: []SkillEntry{}, db: db}
	if db != nil {
		_ = r.loadSkillsFromDB()
	}
	return r
}

func (r *SkillRegistry) Register(e SkillEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e = cloneSkillEntryForStorage(e)
	e.MatchScore = 0
	e.SelectedVariantID = ""
	r.entries = append(r.entries, e)
	r.persistSkillsDBLocked()
}

func (r *SkillRegistry) MergeOrAdd(e SkillEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e = cloneSkillEntryForStorage(e)
	e.MatchScore = 0
	e.SelectedVariantID = ""
	for i := range r.entries {
		if rtutils.StringSlicesEqual(r.entries[i].Capabilities, e.Capabilities) {
			r.entries[i].Attempts += e.Attempts
			r.entries[i].Successes += e.Successes
			if len(e.TriggerEmbedding) > 0 {
				r.entries[i].TriggerEmbedding = append([]float32(nil), e.TriggerEmbedding...)
			}
			for _, nv := range e.Variants {
				mergeSkillVariant(&r.entries[i], nv)
			}
			r.persistSkillsDBLocked()
			return
		}
	}
	r.entries = append(r.entries, e)
	r.persistSkillsDBLocked()
}

func mergeSkillVariant(skill *SkillEntry, nv SkillPlanVariant) {
	if nv.ID == "" || nv.Plan == nil {
		return
	}
	for j := range skill.Variants {
		if skill.Variants[j].ID != nv.ID {
			continue
		}
		skill.Variants[j].Attempts += nv.Attempts
		skill.Variants[j].Successes += nv.Successes
		skill.Variants[j].Plan = CloneSkillPlan(nv.Plan)
		return
	}
	skill.Variants = append(skill.Variants, SkillPlanVariant{
		ID:        nv.ID,
		Plan:      CloneSkillPlan(nv.Plan),
		Attempts:  nv.Attempts,
		Successes: nv.Successes,
	})
}

func (r *SkillRegistry) MarkUsed(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for i := range r.entries {
		if r.entries[i].Name == name {
			r.entries[i].LastUsedAt = now
			break
		}
	}
	r.persistSkillsDBLocked()
}

func BuildSkillEntryFromSession(ctx context.Context, sess *ports.Session, embedder *llmclient.OpenAI) (SkillEntry, bool) {
	if sess == nil || sess.Plan == nil || len(sess.Plan.Steps) == 0 {
		return SkillEntry{}, false
	}
	caps := make([]string, 0, len(sess.Plan.Steps))
	for _, st := range sess.Plan.Steps {
		if st.Capability != "" {
			caps = append(caps, st.Capability)
		}
	}
	if len(caps) == 0 {
		return SkillEntry{}, false
	}
	fp := rtutils.PlanSemanticFingerprint(sess.Plan)
	if fp == "" {
		return SkillEntry{}, false
	}
	var emb []float32
	if embedder != nil && sess.UserInput != "" {
		emb, _ = embedder.Embed(ctx, sess.UserInput)
	}
	return SkillEntry{
		Name:             "learned_" + rtutils.HashPipeJoinedCapabilities(caps),
		Capabilities:     caps,
		Path:             append([]string(nil), caps...),
		TriggerEmbedding: emb,
		Variants: []SkillPlanVariant{{
			ID: fp, Plan: CloneSkillPlan(sess.Plan), Attempts: 1, Successes: 1,
		}},
		Attempts:  1,
		Successes: 1,
	}, true
}

func (r *SkillRegistry) Match(userText string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t := strings.ToLower(userText)
	seen := make(map[string]struct{})
	var out []string
	for _, e := range r.entries {
		if e.tooPoorForMatch() {
			continue
		}
		for _, kw := range e.Keywords {
			if kw != "" && strings.Contains(t, strings.ToLower(kw)) {
				for _, c := range e.Capabilities {
					if _, ok := seen[c]; !ok {
						seen[c] = struct{}{}
						out = append(out, c)
					}
				}
				break
			}
		}
	}
	return out
}

func CloneSkillPlan(p *ports.Plan) *ports.Plan {
	if p == nil {
		return nil
	}
	out := &ports.Plan{Steps: make([]ports.PlanStep, len(p.Steps))}
	for i := range p.Steps {
		out.Steps[i] = p.Steps[i].Clone()
	}
	return out
}

func (r *SkillRegistry) KeywordPlanEntry(userText string) (SkillEntry, bool) {
	t := strings.ToLower(userText)
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := range r.entries {
		e := r.entries[i]
		if e.tooPoorForMatch() || len(e.Variants) == 0 || bestVariantIndex(e.Variants) < 0 {
			continue
		}
		for _, kw := range e.Keywords {
			if kw != "" && strings.Contains(t, strings.ToLower(kw)) {
				return routingSnapshot(e, bestVariantIndex(e.Variants), 0), true
			}
		}
	}
	return SkillEntry{}, false
}

func (r *SkillRegistry) MatchByEmbedding(embedding []float32, k int) []SkillEntry {
	if len(embedding) == 0 || k <= 0 {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	type scored struct {
		entry SkillEntry
		score float64
	}
	var list []scored
	for _, e := range r.entries {
		if e.tooPoorForMatch() || len(e.TriggerEmbedding) == 0 {
			continue
		}
		bi := bestVariantIndex(e.Variants)
		if bi < 0 {
			continue
		}
		sim := rtutils.CosineFloat32(embedding, e.TriggerEmbedding)
		list = append(list, scored{e, sim})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].score > list[j].score })
	if k > len(list) {
		k = len(list)
	}
	out := make([]SkillEntry, 0, k)
	for i := 0; i < k; i++ {
		e := list[i].entry
		bi := bestVariantIndex(e.Variants)
		out = append(out, routingSnapshot(e, bi, list[i].score))
	}
	return out
}

// RecordTurn updates skill stats from keywords / embedding, or from an attributed skill+variant when provided.
func (r *SkillRegistry) RecordTurn(userText string, success bool, queryEmbedding []float32, minEmbeddingSim float64, activeSkillName, activeVariantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.persistSkillsDBLocked()

	if activeSkillName != "" && activeVariantID != "" {
		for i := range r.entries {
			if r.entries[i].Name != activeSkillName {
				continue
			}
			for j := range r.entries[i].Variants {
				if r.entries[i].Variants[j].ID != activeVariantID {
					continue
				}
				r.entries[i].Attempts++
				if success {
					r.entries[i].Successes++
				}
				r.entries[i].Variants[j].Attempts++
				if success {
					r.entries[i].Variants[j].Successes++
				}
				return
			}
			return
		}
	}

	t := strings.ToLower(userText)
	updated := make([]bool, len(r.entries))
	for i := range r.entries {
		for _, kw := range r.entries[i].Keywords {
			if kw == "" {
				continue
			}
			if strings.Contains(t, strings.ToLower(kw)) {
				r.entries[i].Attempts++
				if success {
					r.entries[i].Successes++
				}
				updated[i] = true
				break
			}
		}
	}
	if len(queryEmbedding) == 0 || minEmbeddingSim <= 0 {
		return
	}
	bestI := -1
	var bestSim float64
	for i := range r.entries {
		if updated[i] || len(r.entries[i].TriggerEmbedding) == 0 {
			continue
		}
		sim := rtutils.CosineFloat32(queryEmbedding, r.entries[i].TriggerEmbedding)
		if sim < minEmbeddingSim {
			continue
		}
		if bestI < 0 || sim > bestSim {
			bestSim = sim
			bestI = i
		}
	}
	if bestI >= 0 {
		r.entries[bestI].Attempts++
		if success {
			r.entries[bestI].Successes++
		}
	}
}

func pathOrCaps(e SkillEntry) []string {
	if len(e.Path) > 0 {
		return append([]string(nil), e.Path...)
	}
	return append([]string(nil), e.Capabilities...)
}

func bestVariantIndex(vs []SkillPlanVariant) int {
	if len(vs) == 0 {
		return -1
	}
	best := 0
	for i := 1; i < len(vs); i++ {
		if variantABetter(&vs[i], &vs[best]) {
			best = i
		}
	}
	return best
}

func variantABetter(a, b *SkillPlanVariant) bool {
	if a.Attempts == 0 && b.Attempts > 0 {
		return false
	}
	if b.Attempts == 0 && a.Attempts > 0 {
		return true
	}
	ra, rb := variantRate(a), variantRate(b)
	if ra != rb {
		return ra > rb
	}
	if a.Attempts != b.Attempts {
		return a.Attempts > b.Attempts
	}
	return a.Successes > b.Successes
}

func variantRate(v *SkillPlanVariant) float64 {
	if v.Attempts <= 0 {
		return 0
	}
	return float64(v.Successes) / float64(v.Attempts)
}

func routingSnapshot(e SkillEntry, variantIdx int, matchScore float64) SkillEntry {
	if variantIdx < 0 || variantIdx >= len(e.Variants) {
		return SkillEntry{}
	}
	v := e.Variants[variantIdx]
	out := SkillEntry{
		Name:              e.Name,
		Keywords:          append([]string(nil), e.Keywords...),
		Capabilities:      append([]string(nil), e.Capabilities...),
		Path:              pathOrCaps(e),
		TriggerEmbedding:  nil,
		Attempts:          e.Attempts,
		Successes:         e.Successes,
		LastUsedAt:        e.LastUsedAt,
		MatchScore:        matchScore,
		SelectedVariantID: v.ID,
		Variants:          cloneVariantsDeep(e.Variants),
	}
	return out
}

func cloneVariantsDeep(vs []SkillPlanVariant) []SkillPlanVariant {
	out := make([]SkillPlanVariant, len(vs))
	for i := range vs {
		out[i] = SkillPlanVariant{
			ID: vs[i].ID, Attempts: vs[i].Attempts, Successes: vs[i].Successes,
			Plan: CloneSkillPlan(vs[i].Plan),
		}
	}
	return out
}

func cloneSkillEntryForStorage(e SkillEntry) SkillEntry {
	e.MatchScore = 0
	e.SelectedVariantID = ""
	e.Keywords = append([]string(nil), e.Keywords...)
	e.Capabilities = append([]string(nil), e.Capabilities...)
	e.Path = append([]string(nil), e.Path...)
	e.TriggerEmbedding = append([]float32(nil), e.TriggerEmbedding...)
	e.Variants = cloneVariantsDeep(e.Variants)
	return e
}
