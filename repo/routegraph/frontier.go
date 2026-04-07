package routegraph

import (
	"context"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/OctoSucker/octosucker/store"
)

type hopSignals struct {
	successRate float64
	costScore   float64
	edgeTotal   int
	intentMatch float64
	exploration float64
}

func (g *Graph) hopSignalsRLocked(last, to Node, intent string, totalVisits int) hopSignals {
	var s hopSignals
	s.successRate = 0.5
	k := store.EdgeKey{From: last.String(), To: to.String()}
	e := g.edges[k]
	if e != nil {
		s.edgeTotal = int(e.Success + e.Failure)
		if s.edgeTotal > 0 {
			s.successRate = e.Success / (e.Success + e.Failure)
		}
		if e.Cost > 0 {
			s.costScore = -e.Cost * 0.01
		}
		if e.Latency > 0 {
			s.costScore -= e.Latency * 0.001
		}
	}
	s.intentMatch = g.intentMatchRateRLocked(intent, last.String(), to.String())
	s.exploration = math.Sqrt(math.Log(float64(totalVisits+1)) / float64(s.edgeTotal+1))
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
	f := func(r rune) bool { return unicode.IsSpace(r) || r == ',' || r == '.' }
	for _, w := range strings.FieldsFunc(strings.ToLower(t), f) {
		if len(w) >= 2 {
			m[w] = struct{}{}
		}
	}
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

// Confidence scores how promising the next hop looks from last given the user intent (0..1 heuristic).
func (g *Graph) Confidence(ctx context.Context, intent string, last Node) float64 {
	_ = ctx
	g.mu.RLock()
	defer g.mu.RUnlock()
	next := g.catalogTools
	if len(next) == 0 {
		return 0
	}
	totalVisits := g.totalVisitsUnlocked()
	best := 0.0
	for _, toNPtr := range next {
		if toNPtr == nil {
			continue
		}
		toN := *toNPtr
		s := g.hopSignalsRLocked(last, toN, intent, totalVisits)
		const exploreWeight = 0.09
		combined := s.successRate*0.55 + s.intentMatch*0.27 + s.costScore*0.09 + s.exploration*exploreWeight
		if combined > best {
			best = combined
		}
	}
	return best
}

// Frontier ranks one-hop tool successors from last (nil last means synthetic entry). exclude removes one candidate when set.
func (g *Graph) Frontier(ctx context.Context, intent string, last *Node, exclude *Node) []Node {
	_ = ctx
	g.mu.RLock()
	defer g.mu.RUnlock()
	lastNode := Node{}
	if last != nil {
		lastNode = *last
	}
	next := g.catalogTools
	if len(next) == 0 {
		return nil
	}
	totalVisits := g.totalVisitsUnlocked()
	isEntry := last == nil
	intentW, successW, costW, exploreW := frontierWeights(isEntry)
	type scored struct {
		node  Node
		score float64
	}
	var list []scored
	for _, toNPtr := range next {
		if toNPtr == nil {
			continue
		}
		toN := *toNPtr
		if exclude != nil && toN.Tool == exclude.Tool {
			continue
		}
		s := g.hopSignalsRLocked(lastNode, toN, intent, totalVisits)
		combined := s.successRate*successW + s.intentMatch*intentW + s.costScore*costW + s.exploration*exploreW
		list = append(list, scored{node: toN, score: combined})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].score > list[j].score })
	out := make([]Node, len(list))
	for i := range list {
		out[i] = list[i].node
	}
	return out
}

func frontierWeights(isEntry bool) (intentW, successW, costW, exploreW float64) {
	if isEntry {
		return 0.40, 0.45, 0.08, 0.07
	}
	return 0.20, 0.62, 0.10, 0.08
}

func (g *Graph) totalVisitsUnlocked() int {
	var n int
	for _, e := range g.edges {
		n += int(e.Success + e.Failure)
	}
	return n
}
