package skill

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
)

// SkillRegistry holds learned/static skills and match APIs; SQLite persistence is in skill_storage.go.
type SkillRegistry struct {
	mu       sync.RWMutex
	entries  []SkillEntry
	db       *sql.DB
	Embedder *llmclient.OpenAI
	// VariantExplorationRate is ε for variant selection: with this probability a random usable variant
	// is chosen instead of the scored best (only when ≥2 variants). 0 disables exploration.
	VariantExplorationRate float64
}

// NewSkillRegistry loads skills from db when non-nil.
func NewSkillRegistry(db *sql.DB, embedder *llmclient.OpenAI) (*SkillRegistry, error) {
	r := &SkillRegistry{entries: []SkillEntry{}, db: db, Embedder: embedder, VariantExplorationRate: 0.2}
	if db != nil {
		if err := r.loadSkillsFromDB(); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *SkillRegistry) EmbedText(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	if r.Embedder == nil {
		return nil, fmt.Errorf("skill registry: embedder not configured")
	}
	emb, err := r.Embedder.Embed(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("skill registry: embed text: %w", err)
	}
	return emb, nil
}

func (r *SkillRegistry) MatchByText(ctx context.Context, text string, k int) ([]SkillEntry, error) {
	emb, err := r.EmbedText(ctx, text)
	if err != nil {
		return nil, err
	}
	return r.MatchByEmbedding(emb, k), nil
}

// MatchBestByText returns the best executable embedding hit, or nil when no valid hit exists.
func (r *SkillRegistry) MatchBestByText(ctx context.Context, text string) (*SkillEntry, error) {
	hits, err := r.MatchByText(ctx, text, 1)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 || hits[0].SelectedPlan() == nil {
		return nil, nil
	}
	h := hits[0]
	return &h, nil
}

func (r *SkillRegistry) BuildEntryFromTask(ctx context.Context, t *ports.Task) (SkillEntry, error) {
	return BuildSkillEntryFromTask(ctx, t, r.Embedder)
}

func (r *SkillRegistry) Register(e SkillEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e = cloneSkillEntryForStorage(e)
	e.MatchScore = 0
	e.SelectedVariantID = ""
	r.entries = append(r.entries, e)
	return r.persistSkillsDBLocked()
}

func (r *SkillRegistry) MergeOrAdd(e SkillEntry) error {
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
			return r.persistSkillsDBLocked()
		}
	}
	r.entries = append(r.entries, e)
	return r.persistSkillsDBLocked()
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
		skill.Variants[j].Params = cloneParamSpecs(nv.Params)
		skill.Variants[j].LastUsedUnix = maxInt64(skill.Variants[j].LastUsedUnix, nv.LastUsedUnix)
		return
	}
	skill.Variants = append(skill.Variants, SkillPlanVariant{
		ID:           nv.ID,
		Plan:         CloneSkillPlan(nv.Plan),
		Params:       cloneParamSpecs(nv.Params),
		Attempts:     nv.Attempts,
		Successes:    nv.Successes,
		LastUsedUnix: nv.LastUsedUnix,
	})
}

// MarkUsed updates skill-level LastUsedAt; if variantID is non-empty, also updates that variant's LastUsedUnix.
func (r *SkillRegistry) MarkUsed(skillName, variantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	unix := now.Unix()
	for i := range r.entries {
		if r.entries[i].Name != skillName {
			continue
		}
		r.entries[i].LastUsedAt = now
		if variantID != "" {
			for j := range r.entries[i].Variants {
				if r.entries[i].Variants[j].ID == variantID {
					r.entries[i].Variants[j].LastUsedUnix = unix
					break
				}
			}
		}
		break
	}
	return r.persistSkillsDBLocked()
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

func (r *SkillRegistry) KeywordPlanEntry(userText string) (SkillEntry, bool) {
	t := strings.ToLower(userText)
	now := time.Now()
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := range r.entries {
		e := r.entries[i]
		bi := bestVariantIndex(e.Variants, now, r.VariantExplorationRate)
		if e.tooPoorForMatch() || len(e.Variants) == 0 || bi < 0 {
			continue
		}
		for _, kw := range e.Keywords {
			if kw != "" && strings.Contains(t, strings.ToLower(kw)) {
				return routingSnapshot(e, bi, 0), true
			}
		}
	}
	return SkillEntry{}, false
}

// KeywordPlanHit returns the first keyword-matched entry that has an executable selected plan, or nil.
func (r *SkillRegistry) KeywordPlanHit(userText string) *SkillEntry {
	e, ok := r.KeywordPlanEntry(userText)
	if !ok || e.SelectedPlan() == nil {
		return nil
	}
	return &e
}

func (r *SkillRegistry) MatchByEmbedding(embedding []float32, k int) []SkillEntry {
	if len(embedding) == 0 || k <= 0 {
		return nil
	}
	now := time.Now()
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
		bi := bestVariantIndex(e.Variants, now, r.VariantExplorationRate)
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
		bi := bestVariantIndex(e.Variants, now, r.VariantExplorationRate)
		out = append(out, routingSnapshot(e, bi, list[i].score))
	}
	return out
}

// RecordTurn updates skill stats from keywords / embedding, or from an attributed skill+variant when provided.
func (r *SkillRegistry) RecordTurn(userText string, success bool, queryEmbedding []float32, minEmbeddingSim float64, activeSkillName, activeVariantID string) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer func() {
		if perr := r.persistSkillsDBLocked(); perr != nil {
			err = errors.Join(err, perr)
		}
	}()

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
		return nil
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
	return nil
}
