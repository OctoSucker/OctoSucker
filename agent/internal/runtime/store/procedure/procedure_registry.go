package procedure

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

// ProcedureRegistry holds learned/static procedures and match APIs; SQLite persistence is in procedure_storage.go.
type ProcedureRegistry struct {
	mu       sync.RWMutex
	entries  []ProcedureEntry
	db       *sql.DB
	Embedder *llmclient.OpenAI
	// VariantExplorationRate is ε for variant selection: with this probability a random usable variant
	// is chosen instead of the scored best (only when ≥2 variants). 0 disables exploration.
	VariantExplorationRate float64
}

// NewProcedureRegistry loads procedures from db when non-nil.
func NewProcedureRegistry(db *sql.DB, embedder *llmclient.OpenAI) (*ProcedureRegistry, error) {
	r := &ProcedureRegistry{entries: []ProcedureEntry{}, db: db, Embedder: embedder, VariantExplorationRate: 0.2}
	if db != nil {
		if err := r.loadProceduresFromDB(); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *ProcedureRegistry) EmbedText(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	if r.Embedder == nil {
		return nil, fmt.Errorf("procedure registry: embedder not configured")
	}
	emb, err := r.Embedder.Embed(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("procedure registry: embed text: %w", err)
	}
	return emb, nil
}

func (r *ProcedureRegistry) MatchByText(ctx context.Context, text string, k int) ([]ProcedureEntry, error) {
	emb, err := r.EmbedText(ctx, text)
	if err != nil {
		return nil, err
	}
	return r.MatchByEmbedding(emb, k), nil
}

// MatchBestByText returns the best executable embedding hit, or nil when no valid hit exists.
func (r *ProcedureRegistry) MatchBestByText(ctx context.Context, text string) (*ProcedureEntry, error) {
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

func (r *ProcedureRegistry) BuildEntryFromTask(ctx context.Context, t *ports.Task) (ProcedureEntry, error) {
	return BuildProcedureEntryFromTask(ctx, t, r.Embedder)
}

func (r *ProcedureRegistry) Register(e ProcedureEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e = cloneProcedureEntryForStorage(e)
	e.MatchScore = 0
	e.SelectedVariantID = ""
	r.entries = append(r.entries, e)
	return r.persistProceduresDBLocked()
}

func (r *ProcedureRegistry) MergeOrAdd(e ProcedureEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e = cloneProcedureEntryForStorage(e)
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
				mergeProcedureVariant(&r.entries[i], nv)
			}
			return r.persistProceduresDBLocked()
		}
	}
	r.entries = append(r.entries, e)
	return r.persistProceduresDBLocked()
}

func mergeProcedureVariant(procedure *ProcedureEntry, nv ProcedurePlanVariant) {
	if nv.ID == "" || nv.Plan == nil {
		return
	}
	for j := range procedure.Variants {
		if procedure.Variants[j].ID != nv.ID {
			continue
		}
		procedure.Variants[j].Attempts += nv.Attempts
		procedure.Variants[j].Successes += nv.Successes
		procedure.Variants[j].Plan = CloneProcedurePlan(nv.Plan)
		procedure.Variants[j].Params = cloneParamSpecs(nv.Params)
		procedure.Variants[j].LastUsedUnix = maxInt64(procedure.Variants[j].LastUsedUnix, nv.LastUsedUnix)
		return
	}
	procedure.Variants = append(procedure.Variants, ProcedurePlanVariant{
		ID:           nv.ID,
		Plan:         CloneProcedurePlan(nv.Plan),
		Params:       cloneParamSpecs(nv.Params),
		Attempts:     nv.Attempts,
		Successes:    nv.Successes,
		LastUsedUnix: nv.LastUsedUnix,
	})
}

// MarkUsed updates procedure-level LastUsedAt; if variantID is non-empty, also updates that variant's LastUsedUnix.
func (r *ProcedureRegistry) MarkUsed(procedureName, variantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	unix := now.Unix()
	for i := range r.entries {
		if r.entries[i].Name != procedureName {
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
	return r.persistProceduresDBLocked()
}

func (r *ProcedureRegistry) Match(userText string) []string {
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

func (r *ProcedureRegistry) KeywordPlanEntry(userText string) (ProcedureEntry, bool) {
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
	return ProcedureEntry{}, false
}

// KeywordPlanHit returns the first keyword-matched entry that has an executable selected plan, or nil.
func (r *ProcedureRegistry) KeywordPlanHit(userText string) *ProcedureEntry {
	e, ok := r.KeywordPlanEntry(userText)
	if !ok || e.SelectedPlan() == nil {
		return nil
	}
	return &e
}

func (r *ProcedureRegistry) MatchByEmbedding(embedding []float32, k int) []ProcedureEntry {
	if len(embedding) == 0 || k <= 0 {
		return nil
	}
	now := time.Now()
	r.mu.RLock()
	defer r.mu.RUnlock()
	type scored struct {
		entry ProcedureEntry
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
	out := make([]ProcedureEntry, 0, k)
	for i := 0; i < k; i++ {
		e := list[i].entry
		bi := bestVariantIndex(e.Variants, now, r.VariantExplorationRate)
		out = append(out, routingSnapshot(e, bi, list[i].score))
	}
	return out
}

// RecordTurn updates procedure stats from keywords / embedding, or from an attributed procedure+variant when provided.
func (r *ProcedureRegistry) RecordTurn(userText string, success bool, queryEmbedding []float32, minEmbeddingSim float64, activeProcedureName, activeVariantID string) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer func() {
		if perr := r.persistProceduresDBLocked(); perr != nil {
			err = errors.Join(err, perr)
		}
	}()

	if activeProcedureName != "" && activeVariantID != "" {
		for i := range r.entries {
			if r.entries[i].Name != activeProcedureName {
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
