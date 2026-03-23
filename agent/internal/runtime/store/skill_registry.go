package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

type SkillEntry struct {
	Name             string
	Keywords         []string
	Capabilities     []string
	Path             []string
	Plan             *ports.Plan
	TriggerEmbedding []float32
	Attempts         int
	Successes        int
	LastUsedAt       time.Time
	MatchScore       float64
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

type SkillRegistry struct {
	mu      sync.RWMutex
	entries []SkillEntry
}

func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{}
}

func (r *SkillRegistry) Register(e SkillEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e.MatchScore = 0
	r.entries = append(r.entries, e)
}

func (r *SkillRegistry) MergeOrAdd(e SkillEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e.MatchScore = 0
	for i := range r.entries {
		if capabilitySliceEqual(r.entries[i].Capabilities, e.Capabilities) {
			r.entries[i].Attempts += e.Attempts
			r.entries[i].Successes += e.Successes
			if len(e.TriggerEmbedding) > 0 {
				r.entries[i].TriggerEmbedding = append([]float32(nil), e.TriggerEmbedding...)
			}
			if e.Plan != nil {
				r.entries[i].Plan = CloneSkillPlan(e.Plan)
			}
			return
		}
	}
	r.entries = append(r.entries, e)
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
}

func hashCapabilities(caps []string) string {
	h := sha256.Sum256([]byte(strings.Join(caps, "|")))
	return hex.EncodeToString(h[:])[:8]
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
	var emb []float32
	if embedder != nil && sess.UserInput != "" {
		emb, _ = embedder.Embed(ctx, sess.UserInput)
	}
	return SkillEntry{
		Name:             "learned_" + hashCapabilities(caps),
		Capabilities:     caps,
		Path:             append([]string(nil), caps...),
		Plan:             CloneSkillPlan(sess.Plan),
		TriggerEmbedding: emb,
		Attempts:         1,
		Successes:        1,
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

func (e SkillEntry) PreferredPath() []string {
	return pathOrCaps(e)
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
		if e.tooPoorForMatch() || e.Plan == nil {
			continue
		}
		for _, kw := range e.Keywords {
			if kw != "" && strings.Contains(t, strings.ToLower(kw)) {
				return e, true
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
		sim := cosine(embedding, e.TriggerEmbedding)
		list = append(list, scored{e, sim})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].score > list[j].score })
	if k > len(list) {
		k = len(list)
	}
	out := make([]SkillEntry, 0, k)
	for i := 0; i < k; i++ {
		e := list[i].entry
		out = append(out, SkillEntry{
			Name:             e.Name,
			Keywords:         append([]string(nil), e.Keywords...),
			Capabilities:     append([]string(nil), e.Capabilities...),
			Path:             pathOrCaps(e),
			Plan:             CloneSkillPlan(e.Plan),
			TriggerEmbedding: nil,
			Attempts:         e.Attempts,
			Successes:        e.Successes,
			LastUsedAt:       e.LastUsedAt,
			MatchScore:       list[i].score,
		})
	}
	return out
}

func (r *SkillRegistry) RecordTurn(userText string, success bool, queryEmbedding []float32, minEmbeddingSim float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
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
		sim := cosine(queryEmbedding, r.entries[i].TriggerEmbedding)
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

func capabilitySliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func pathOrCaps(e SkillEntry) []string {
	if len(e.Path) > 0 {
		return append([]string(nil), e.Path...)
	}
	return append([]string(nil), e.Capabilities...)
}
