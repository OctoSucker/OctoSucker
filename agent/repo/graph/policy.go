package graph

import (
	"context"
	"math"
	"sort"
)

type FrontierSortStrategy string

const (
	FrontierSortAuto               FrontierSortStrategy = "auto"
	FrontierSortIntentHeavy        FrontierSortStrategy = "intent_heavy"
	FrontierSortTransitionBalanced FrontierSortStrategy = "transition_balanced"
)

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
	for _, toNPtr := range next {
		if toNPtr == nil {
			continue
		}
		toN := *toNPtr
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

// Frontier ranks one-hop successors (or entries).
// last=nil means entry node. exclude is optional and removed from candidates when set.
func (g *Graph) Frontier(ctx context.Context, intent string, last *Node, exclude *Node, strategy FrontierSortStrategy) ([]Node, error) {
	_ = ctx
	g.mu.RLock()
	defer g.mu.RUnlock()
	lastNode := Node{}
	if last != nil {
		lastNode = *last
	}
	next := g.staticSuccessorsLocked(lastNode)
	if len(next) == 0 {
		return nil, nil
	}
	totalVisits := totalVisitsLocked(g)
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
		if exclude != nil && exclude.IsValid() && toN == *exclude {
			continue
		}
		k := Key{From: lastNode, To: toN}
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
		ctxScore := similarIntentScoreLocked(g, intent, lastNode.String(), toN.String())
		exploration := 0.0
		if totalVisits >= 0 {
			exploration = math.Sqrt(math.Log(float64(totalVisits+1)) / float64(edgeTotal+1))
		}
		explorationDecay := 1.0
		if g.totalRuns > 0 {
			decay := math.Exp(-0.001 * float64(g.totalRuns))
			if decay < 0.02 {
				decay = 0.02
			}
			explorationDecay = decay
		}
		intentW, successW, costW, exploreW := frontierWeights(strategy, last == nil)
		combined := successRate*successW + ctxScore*intentW + costScore*costW + exploration*exploreW*explorationDecay
		list = append(list, scored{node: toN, score: combined})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].score > list[j].score })
	out := make([]Node, len(list))
	for i := range list {
		out[i] = list[i].node
	}
	return out, nil
}

func frontierWeights(strategy FrontierSortStrategy, isEntry bool) (intentW, successW, costW, exploreW float64) {
	switch strategy {
	case FrontierSortIntentHeavy:
		return 0.45, 0.40, 0.08, 0.07
	case FrontierSortTransitionBalanced:
		return 0.18, 0.64, 0.10, 0.08
	case FrontierSortAuto, "":
		if isEntry {
			return 0.40, 0.45, 0.08, 0.07
		}
		return 0.20, 0.62, 0.10, 0.08
	default:
		return 0.20, 0.62, 0.10, 0.08
	}
}
