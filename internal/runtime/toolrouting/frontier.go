package toolrouting

import (
	"context"
	"sort"
	"strings"
	"unicode"

	"github.com/OctoSucker/octosucker/internal/storage"
)

type hopSignals struct {
	successRate float64
	edgeTotal   int
	intentMatch float64
}

func (g *Graph) hopSignalsRLocked(last, to Node, intent string) hopSignals {
	var s hopSignals
	s.successRate = 0.5
	k := storage.EdgeKey{From: last.String(), To: to.String()}
	e := g.edges[k]
	if e != nil {
		s.edgeTotal = int(e.Success + e.Failure)
		if s.edgeTotal > 0 {
			s.successRate = e.Success / (e.Success + e.Failure)
		}
	}
	s.intentMatch = g.intentMatchRateRLocked(intent, last.String(), to.String())
	return s
}

func (g *Graph) intentMatchRateRLocked(intent, from, to string) float64 {
	if intent == "" {
		return 0
	}
	if len(g.recentTransitions) == 0 {
		return 0
	}
	iw := intentWordSet(intent)
	if len(iw) == 0 {
		return 0
	}
	var success, total int
	for _, t := range g.recentTransitions {
		if t.From != from || t.To != to {
			continue
		}
		tw := intentWordSet(t.Intent)
		if wordOverlapRatio(iw, tw) < 0.2 {
			continue
		}
		total++
		if t.Outcome {
			success++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(success) / float64(total)
}

func intentWordSet(t string) map[string]struct{} {
	m := make(map[string]struct{})
	var latin []rune
	var cjk []rune
	flushLatin := func() {
		if len(latin) >= 2 {
			m[string(latin)] = struct{}{}
		}
		latin = latin[:0]
	}
	flushCJK := func() {
		if len(cjk) == 1 {
			m[string(cjk)] = struct{}{}
		}
		for i := 0; i+1 < len(cjk); i++ {
			m[string(cjk[i:i+2])] = struct{}{}
		}
		cjk = cjk[:0]
	}
	for _, r := range []rune(strings.ToLower(t)) {
		switch {
		case unicode.Is(unicode.Han, r):
			flushLatin()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			latin = append(latin, r)
		default:
			flushLatin()
			flushCJK()
		}
	}
	flushLatin()
	flushCJK()
	return m
}

func wordOverlapRatio(a, b map[string]struct{}) float64 {
	if len(a) == 0 {
		return 0
	}
	n := 0
	for k := range a {
		if _, ok := b[k]; ok {
			n++
		}
	}
	return float64(n) / float64(len(a))
}

// Recommend returns only well-supported historical tools. It is intentionally
// conservative because the planner treats this output as advice, not routing.
func (g *Graph) Recommend(ctx context.Context, intent, previousTool string, limit int) []string {
	_ = ctx
	if g == nil || limit <= 0 {
		return nil
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	from := Node{Tool: strings.TrimSpace(previousTool)}
	type candidate struct {
		tool  string
		score float64
	}
	var candidates []candidate
	for _, node := range g.catalogTools {
		signals := g.hopSignalsRLocked(from, node, intent)
		if signals.edgeTotal < 2 || signals.successRate < 0.67 || signals.intentMatch < 0.20 {
			continue
		}
		score := signals.successRate*0.65 + signals.intentMatch*0.35
		candidates = append(candidates, candidate{tool: node.Tool, score: score})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.tool)
	}
	return out
}
