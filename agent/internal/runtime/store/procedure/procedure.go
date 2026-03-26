package procedure

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
)

// ErrNoProcedureFromTask means the task has no extractable capability path / fingerprint for procedure learning.
var ErrNoProcedureFromTask = errors.New("procedure: no extractable procedure from task")

// ProcedurePlanVariant is the "Variant → Plan" layer:
// same macro capability path (see ProcedureEntry) may have multiple concrete plans + runtime param schema.
// Layer chain: Intent (task) → ProcedureEntry (trigger + path) → ProcedurePlanVariant → ports.Plan → PlanStep → tool.
type ProcedurePlanVariant struct {
	// --- Variant identity ---
	ID string

	// --- Executable plan + parameterization (template → instantiated in planner) ---
	Plan   *ports.Plan
	Params []ProcedureParamSpec

	// --- Variant-level learning / scheduling ---
	Attempts     int
	Successes    int
	LastUsedUnix int64 // unix seconds when this variant was last chosen (0 = never); drives recency decay in selection
}

// ProcedureParamSpec describes one runtime argument accepted by a procedure variant.
type ProcedureParamSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
	// FromStepID: if set, fill this param from aggregated trace summary for that plan step id (after BuildInvocationArgs / InvokeProcedureVariant).
	FromStepID string `json:"from_step_id,omitempty"`
}

// ProcedureEntry is the persisted "Procedure" layer: trigger + macro capability path + variants.
// It does not hold user intent text; matching happens in ProcedureRegistry / Planner using keywords and embeddings.
// Layer chain: User intent → ProcedureRegistry.Match* / MatchByEmbedding → ProcedureEntry → ProcedurePlanVariant → Plan → Steps.
type ProcedureEntry struct {
	// --- Procedure identity (storage key, learned_*, etc.) ---
	Name string

	// --- Trigger layer (how this procedure is recalled from intent) ---
	Keywords         []string
	TriggerEmbedding []float32 // cosine-matched to query embedding when non-empty

	// --- Macro procedure: capability sequence (abstract path; may differ slightly from a variant's concrete steps) ---
	Capabilities []string
	Path         []string // preferred routing hint; falls back to Capabilities when empty

	// --- Variant layer: competing implementations + stats ---
	Variants []ProcedurePlanVariant

	// --- Procedure-level aggregates (keyword hits, embedding reinforcement, attributed turns) ---
	Attempts   int
	Successes  int
	LastUsedAt time.Time

	// --- Ephemeral routing snapshot (not persisted on Register/Merge) ---
	MatchScore        float64 // embedding similarity etc., set only when returning a hit to the planner
	SelectedVariantID string  `json:"-"` // if set, SelectedVariant/SelectedPlan use this variant; else bestVariantIndex
}

func (e ProcedureEntry) SuccessRate() float64 {
	if e.Attempts <= 0 {
		return 1
	}
	return float64(e.Successes) / float64(e.Attempts)
}

func (e ProcedureEntry) tooPoorForMatch() bool {
	return e.Attempts > 5 && e.SuccessRate() < 0.3
}

// SelectedPlan returns a deep copy of the plan for the selected or best-scoring variant.
func (e ProcedureEntry) SelectedPlan() *ports.Plan {
	v := e.SelectedVariant()
	if v == nil {
		return nil
	}
	return CloneProcedurePlan(v.Plan)
}

// SelectedVariant returns a deep copy of selected (or best) variant.
func (e ProcedureEntry) SelectedVariant() *ProcedurePlanVariant {
	if len(e.Variants) == 0 {
		return nil
	}
	if e.SelectedVariantID != "" {
		for i := range e.Variants {
			if e.Variants[i].ID == e.SelectedVariantID {
				v := cloneVariantDeep(e.Variants[i])
				return &v
			}
		}
	}
	bi := bestVariantIndex(e.Variants, time.Now(), 0)
	if bi < 0 {
		return nil
	}
	v := cloneVariantDeep(e.Variants[bi])
	return &v
}

func (e ProcedureEntry) PreferredPath() []string {
	return pathOrCaps(e)
}

// ProcedureLearnCapKeyFromTask returns a stable key for the capability sequence (same as learned_* name suffix)
// and the number of plan steps. ok is false if there is no usable capability path.
func ProcedureLearnCapKeyFromTask(taskState *ports.Task) (capKey string, planStepCount int, ok bool) {
	if taskState == nil || taskState.Plan == nil || len(taskState.Plan.Steps) == 0 {
		return "", 0, false
	}
	planStepCount = len(taskState.Plan.Steps)
	caps := make([]string, 0, len(taskState.Plan.Steps))
	for _, st := range taskState.Plan.Steps {
		if st.Capability != "" {
			caps = append(caps, st.Capability)
		}
	}
	if len(caps) == 0 {
		return "", planStepCount, false
	}
	return rtutils.HashPipeJoinedCapabilities(caps), planStepCount, true
}

func BuildProcedureEntryFromTask(ctx context.Context, taskState *ports.Task, embedder *llmclient.OpenAI) (ProcedureEntry, error) {
	if taskState == nil || taskState.Plan == nil || len(taskState.Plan.Steps) == 0 {
		return ProcedureEntry{}, ErrNoProcedureFromTask
	}
	caps := make([]string, 0, len(taskState.Plan.Steps))
	for _, st := range taskState.Plan.Steps {
		if st.Capability != "" {
			caps = append(caps, st.Capability)
		}
	}
	if len(caps) == 0 {
		return ProcedureEntry{}, ErrNoProcedureFromTask
	}
	fp := rtutils.PlanSemanticFingerprint(taskState.Plan)
	if fp == "" {
		return ProcedureEntry{}, ErrNoProcedureFromTask
	}
	var emb []float32
	if embedder != nil && taskState.UserInput.Text != "" {
		var err error
		emb, err = embedder.Embed(ctx, taskState.UserInput.Text)
		if err != nil {
			return ProcedureEntry{}, fmt.Errorf("procedure: embed user input: %w", err)
		}
	}
	now := time.Now().Unix()
	return ProcedureEntry{
		Name:             "learned_" + rtutils.HashPipeJoinedCapabilities(caps),
		Capabilities:     caps,
		Path:             append([]string(nil), caps...),
		TriggerEmbedding: emb,
		Variants: []ProcedurePlanVariant{{
			ID: fp, Plan: CloneProcedurePlan(taskState.Plan), Attempts: 1, Successes: 1, LastUsedUnix: now,
		}},
		Attempts:  1,
		Successes: 1,
	}, nil
}

func CloneProcedurePlan(p *ports.Plan) *ports.Plan {
	if p == nil {
		return nil
	}
	out := &ports.Plan{Steps: make([]ports.PlanStep, len(p.Steps))}
	for i := range p.Steps {
		out.Steps[i] = p.Steps[i].Clone()
	}
	return out
}

func pathOrCaps(e ProcedureEntry) []string {
	if len(e.Path) > 0 {
		return append([]string(nil), e.Path...)
	}
	return append([]string(nil), e.Capabilities...)
}

// procedureVariantRecencyHalfLife is the time after which a variant's success-rate is weighted halfway toward procedureVariantRecencyFloor.
const procedureVariantRecencyHalfLife = 7 * 24 * time.Hour
const procedureVariantRecencyFloor = 0.55

func bestVariantIndex(vs []ProcedurePlanVariant, now time.Time, exploreRate float64) int {
	if len(vs) == 0 {
		return -1
	}
	usable := usableVariantIndices(vs)
	if len(usable) == 0 {
		return -1
	}
	if exploreRate > 0 && len(usable) > 1 && rand.Float64() < exploreRate {
		return usable[rand.Intn(len(usable))]
	}
	best := usable[0]
	for _, i := range usable[1:] {
		if variantABetter(&vs[i], &vs[best], now) {
			best = i
		}
	}
	return best
}

// usableVariantIndices returns indices with a non-empty ID and non-nil Plan (same notion as merge/ persist).
func usableVariantIndices(vs []ProcedurePlanVariant) []int {
	var out []int
	for i := range vs {
		if vs[i].ID != "" && vs[i].Plan != nil {
			out = append(out, i)
		}
	}
	return out
}

func variantABetter(a, b *ProcedurePlanVariant, now time.Time) bool {
	if a.Attempts == 0 && b.Attempts > 0 {
		return false
	}
	if b.Attempts == 0 && a.Attempts > 0 {
		return true
	}
	sa, sb := variantCompositeScore(a, now), variantCompositeScore(b, now)
	const eps = 1e-9
	if math.Abs(sa-sb) > eps {
		return sa > sb
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

func variantCompositeScore(v *ProcedurePlanVariant, now time.Time) float64 {
	return variantRate(v) * variantRecencyMultiplier(v.LastUsedUnix, now)
}

func variantRecencyMultiplier(lastUsedUnix int64, now time.Time) float64 {
	if lastUsedUnix <= 0 {
		return 1
	}
	age := now.Sub(time.Unix(lastUsedUnix, 0))
	if age < 0 {
		age = 0
	}
	hSec := procedureVariantRecencyHalfLife.Seconds()
	if hSec <= 0 {
		return 1
	}
	decay := math.Exp(-math.Ln2 * age.Seconds() / hSec)
	return procedureVariantRecencyFloor + (1-procedureVariantRecencyFloor)*decay
}

func variantRate(v *ProcedurePlanVariant) float64 {
	if v.Attempts <= 0 {
		return 0
	}
	return float64(v.Successes) / float64(v.Attempts)
}

func routingSnapshot(e ProcedureEntry, variantIdx int, matchScore float64) ProcedureEntry {
	if variantIdx < 0 || variantIdx >= len(e.Variants) {
		return ProcedureEntry{}
	}
	v := e.Variants[variantIdx]
	out := ProcedureEntry{
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

func cloneVariantsDeep(vs []ProcedurePlanVariant) []ProcedurePlanVariant {
	out := make([]ProcedurePlanVariant, len(vs))
	for i := range vs {
		out[i] = cloneVariantDeep(vs[i])
	}
	return out
}

func cloneVariantDeep(v ProcedurePlanVariant) ProcedurePlanVariant {
	return ProcedurePlanVariant{
		ID:           v.ID,
		Attempts:     v.Attempts,
		Successes:    v.Successes,
		LastUsedUnix: v.LastUsedUnix,
		Plan:         CloneProcedurePlan(v.Plan),
		Params:       cloneParamSpecs(v.Params),
	}
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func cloneParamSpecs(in []ProcedureParamSpec) []ProcedureParamSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]ProcedureParamSpec, len(in))
	copy(out, in)
	return out
}

// BuildProcedureInvocationArgs fills args via BuildInvocationContext with user text only (no trace).
// Prefer InvokeProcedureVariant when trace / from_step_id matter.
func BuildProcedureInvocationArgs(userText string, params []ProcedureParamSpec) (map[string]any, error) {
	return BuildInvocationArgs(InvocationContext{UserInput: userText}, params)
}

// InstantiateProcedurePlan renders {{arg_name}} placeholders in step arguments.
func InstantiateProcedurePlan(plan *ports.Plan, args map[string]any) *ports.Plan {
	out := CloneProcedurePlan(plan)
	if out == nil || len(args) == 0 {
		return out
	}
	for i := range out.Steps {
		out.Steps[i].Arguments = renderArgumentMap(out.Steps[i].Arguments, args)
	}
	return out
}

func renderArgumentMap(in map[string]any, args map[string]any) map[string]any {
	if len(in) == 0 {
		return in
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = renderAny(v, args)
	}
	return out
}

func renderAny(v any, args map[string]any) any {
	switch t := v.(type) {
	case string:
		return renderStringTemplate(t, args)
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = renderAny(t[i], args)
		}
		return out
	case map[string]any:
		return renderArgumentMap(t, args)
	default:
		return v
	}
}

func renderStringTemplate(s string, args map[string]any) any {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "{{") && strings.HasSuffix(trimmed, "}}") && strings.Count(trimmed, "{{") == 1 && strings.Count(trimmed, "}}") == 1 {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "{{"), "}}"))
		if val, ok := args[name]; ok {
			return val
		}
		return s
	}
	out := s
	for k, v := range args {
		if k == "" {
			continue
		}
		out = strings.ReplaceAll(out, "{{"+k+"}}", fmt.Sprint(v))
	}
	return out
}

func cloneProcedureEntryForStorage(e ProcedureEntry) ProcedureEntry {
	e.MatchScore = 0
	e.SelectedVariantID = ""
	e.Keywords = append([]string(nil), e.Keywords...)
	e.Capabilities = append([]string(nil), e.Capabilities...)
	e.Path = append([]string(nil), e.Path...)
	e.TriggerEmbedding = append([]float32(nil), e.TriggerEmbedding...)
	e.Variants = cloneVariantsDeep(e.Variants)
	return e
}
