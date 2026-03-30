package graph

import (
	"context"
	"math"
	"sort"
)

const globalDistInf = 1e18

func similarIntentScoreLocked(g *Graph, intent, from, to string) float64 {
	if intent == "" {
		return 0
	}
	recent := g.recentTransitionsCloneLocked()
	if len(recent) == 0 {
		return 0
	}
	iw := routingIntentWordSet(intent)
	if len(iw) == 0 {
		return 0
	}
	var success, total int
	for _, t := range recent {
		if t.From != from || t.To != to {
			continue
		}
		tw := routingIntentWordSet(t.Intent)
		if routingWordOverlapRatio(iw, tw) < 0.2 {
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

func edgeWeightGlobalLocked(g *Graph, from, to Node) float64 {
	k := Key{From: from, To: to}
	e := g.edges[k]
	p := 0.5
	if e != nil && e.Success+e.Failure > 0 {
		p = e.Success / (e.Success + e.Failure)
	}
	w := 1.0 - p
	if e != nil {
		if e.Latency > 0 {
			w += e.Latency * 0.001
		}
		if e.Cost > 0 {
			w += e.Cost * 0.01
		}
	}
	if w < 1e-9 {
		w = 1e-9
	}
	return w
}

func immediateSuccessorSetLocked(g *Graph, last Node) map[string]struct{} {
	next := g.staticSuccessorsLocked(last)
	m := make(map[string]struct{}, len(next))
	for _, toN := range next {
		m[toN.String()] = struct{}{}
	}
	return m
}

func totalVisitsLocked(g *Graph) int {
	var n int
	for _, e := range g.edges {
		n += int(e.Success + e.Failure)
	}
	return n
}

// Confidence estimates routing quality from last toward one-hop successors (or entries).
func (g *Graph) Confidence(ctx context.Context, intent string, last Node) float64 {
	_ = ctx
	g.mu.RLock()
	defer g.mu.RUnlock()
	next := g.staticSuccessorsLocked(last)
	if len(next) == 0 {
		return 0
	}
	totalVisits := totalVisitsLocked(g)
	best := 0.0
	for _, toN := range next {
		k := Key{From: last, To: toN}
		e := g.edges[k]
		successRate := 0.5
		costScore := 0.0
		edgeTotal := 0
		if e != nil {
			edgeTotal = int(e.Success + e.Failure)
			if edgeTotal > 0 {
				successRate = e.Success / (e.Success + e.Failure)
			}
			if e.Cost > 0 {
				costScore = -e.Cost * 0.01
			}
			if e.Latency > 0 {
				costScore -= e.Latency * 0.001
			}
		}
		ctxScore := similarIntentScoreLocked(g, intent, last.String(), toN.String())
		exploration := 0.0
		if totalVisits >= 0 {
			exploration = math.Sqrt(math.Log(float64(totalVisits+1)) / float64(edgeTotal+1))
		}
		explorationWeight := 0.09
		if g.totalRuns > 0 {
			decay := math.Exp(-0.001 * float64(g.totalRuns))
			if decay < 0.02 {
				decay = 0.02
			}
			explorationWeight = 0.09 * decay
		}
		combined := successRate*0.55 + ctxScore*0.27 + costScore*0.09 + exploration*explorationWeight
		if combined > best {
			best = combined
		}
	}
	return best
}

// Frontier ranks one-hop successors (or entries) by the same heuristic as Confidence.
// lastSuccess is reserved for outcome-conditioned expansion; currently unused.
func (g *Graph) Frontier(ctx context.Context, intent string, last Node, lastSuccess bool) ([]Node, error) {
	_ = ctx
	_ = lastSuccess
	g.mu.RLock()
	defer g.mu.RUnlock()
	next := g.staticSuccessorsLocked(last)
	if len(next) == 0 {
		return nil, nil
	}
	totalVisits := totalVisitsLocked(g)
	type scored struct {
		node  Node
		score float64
	}
	var list []scored
	for _, toN := range next {
		k := Key{From: last, To: toN}
		e := g.edges[k]
		successRate := 0.5
		costScore := 0.0
		edgeTotal := 0
		if e != nil {
			edgeTotal = int(e.Success + e.Failure)
			if edgeTotal > 0 {
				successRate = e.Success / (e.Success + e.Failure)
			}
			if e.Cost > 0 {
				costScore = -e.Cost * 0.01
			}
			if e.Latency > 0 {
				costScore -= e.Latency * 0.001
			}
		}
		ctxScore := similarIntentScoreLocked(g, intent, last.String(), toN.String())
		exploration := 0.0
		if totalVisits >= 0 {
			exploration = math.Sqrt(math.Log(float64(totalVisits+1)) / float64(edgeTotal+1))
		}
		explorationWeight := 0.09
		if g.totalRuns > 0 {
			decay := math.Exp(-0.001 * float64(g.totalRuns))
			if decay < 0.02 {
				decay = 0.02
			}
			explorationWeight = 0.09 * decay
		}
		combined := successRate*0.55 + ctxScore*0.27 + costScore*0.09 + exploration*explorationWeight
		list = append(list, scored{node: toN, score: combined})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].score > list[j].score })
	out := make([]Node, len(list))
	for i := range list {
		out[i] = list[i].node
	}
	return out, nil
}

// FilterCandidatesOnImmediateEdge keeps only candidates that are direct static successors of last.
func (g *Graph) FilterCandidatesOnImmediateEdge(last Node, candidates []Node) []Node {
	if len(candidates) == 0 {
		return nil
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	nextFromLast := immediateSuccessorSetLocked(g, last)
	out := make([]Node, 0, len(candidates))
	for _, c := range candidates {
		if _, ok := nextFromLast[c.String()]; ok {
			out = append(out, c)
		}
	}
	return out
}

// PickBestByImmediateEdge returns the feasible candidate with minimal immediate edge weight among
// candidates on a static one-hop edge from last.
func (g *Graph) PickBestByImmediateEdge(ctx context.Context, intent string, last Node, candidates []Node) (Node, bool) {
	_ = ctx
	_ = intent
	if len(candidates) == 0 {
		return Node{}, false
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	nextFromLast := immediateSuccessorSetLocked(g, last)
	bestC := Node{}
	bestWeight := globalDistInf
	for _, c := range candidates {
		if _, ok := nextFromLast[c.String()]; !ok {
			continue
		}
		w := edgeWeightGlobalLocked(g, last, c)
		if w < bestWeight {
			bestWeight = w
			bestC = c
		}
	}
	if !bestC.IsValid() {
		return Node{}, false
	}
	return bestC, true
}
