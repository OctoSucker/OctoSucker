package store

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/OctoSucker/agent/pkg/ports"
)

const recentTransitionsCap = 200
const trajectoryGamma = 0.9

type RoutingGraph struct {
	mu                sync.RWMutex
	edges             map[edgeKey]*portsEdge
	static            map[string][]string
	recentTransitions []contextTransition
	totalRuns         int64
}

type EdgeStat struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Success int    `json:"success"`
	Failure int    `json:"failure"`
}

type edgeKey struct {
	from string
	to   string
}

type contextTransition struct {
	Intent  string
	From    string
	To      string
	Outcome int
}

type portsEdge struct {
	Success float64
	Failure float64
	Cost    float64
	Latency float64
}

func NewRoutingGraphFromCapabilities(m map[string]ports.Capability) *RoutingGraph {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	static := make(map[string][]string, len(ids)+1)
	static[""] = append([]string(nil), ids...)
	_, hasFinish := m["finish"]
	for _, id := range ids {
		if id == "finish" {
			static[id] = []string{}
			continue
		}
		if hasFinish {
			static[id] = []string{"finish"}
		} else {
			static[id] = []string{}
		}
	}
	return &RoutingGraph{edges: make(map[edgeKey]*portsEdge), static: static}
}

func (s *RoutingGraph) Confidence(ctx context.Context, rc ports.RoutingContext, last string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	next := s.static[last]
	if len(next) == 0 {
		next, _ = s.entryNodesLocked()
	}
	if len(next) == 0 {
		return 0
	}
	totalVisits := s.totalVisitsLocked()
	best := 0.0
	for _, to := range next {
		k := edgeKey{from: last, to: to}
		e := s.edges[k]
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
		ctxScore := s.similarIntentScoreLocked(rc.IntentText, last, to)
		exploration := 0.0
		if totalVisits >= 0 {
			exploration = math.Sqrt(math.Log(float64(totalVisits+1)) / float64(edgeTotal+1))
		}
		explorationWeight := 0.09
		if s.totalRuns > 0 {
			decay := math.Exp(-0.001 * float64(s.totalRuns))
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

func (s *RoutingGraph) EntryNodes(ctx context.Context, rc ports.RoutingContext) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.static[""]...), nil
}

func (s *RoutingGraph) totalVisitsLocked() int {
	var n int
	for _, e := range s.edges {
		n += int(e.Success + e.Failure)
	}
	return n
}

func (s *RoutingGraph) Frontier(ctx context.Context, rc ports.RoutingContext, last string, outcome int) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	next := s.static[last]
	if len(next) == 0 {
		next, _ = s.entryNodesLocked()
	}
	if len(next) == 0 {
		return nil, nil
	}
	totalVisits := s.totalVisitsLocked()
	type scored struct {
		cap   string
		score float64
	}
	var list []scored
	for _, to := range next {
		k := edgeKey{from: last, to: to}
		e := s.edges[k]
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
		ctxScore := s.similarIntentScoreLocked(rc.IntentText, last, to)
		exploration := 0.0
		if totalVisits >= 0 {
			exploration = math.Sqrt(math.Log(float64(totalVisits+1)) / float64(edgeTotal+1))
		}
		explorationWeight := 0.09
		if s.totalRuns > 0 {
			decay := math.Exp(-0.001 * float64(s.totalRuns))
			if decay < 0.02 {
				decay = 0.02
			}
			explorationWeight = 0.09 * decay
		}
		combined := successRate*0.55 + ctxScore*0.27 + costScore*0.09 + exploration*explorationWeight
		list = append(list, scored{to, combined})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].score > list[j].score })
	out := make([]string, len(list))
	for i := range list {
		out[i] = list[i].cap
	}
	return out, nil
}

func (s *RoutingGraph) entryNodesLocked() ([]string, error) {
	return append([]string(nil), s.static[""]...), nil
}

func (s *RoutingGraph) similarIntentScoreLocked(intent, from, to string) float64 {
	if intent == "" || len(s.recentTransitions) == 0 {
		return 0
	}
	iw := intentWords(intent)
	if len(iw) == 0 {
		return 0
	}
	var success, total int
	for _, t := range s.recentTransitions {
		if t.From != from || t.To != to {
			continue
		}
		tw := intentWords(t.Intent)
		if wordOverlap(iw, tw) < 0.2 {
			continue
		}
		total++
		if t.Outcome == 0 {
			success++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(success) / float64(total)
}

func (s *RoutingGraph) RecordTransition(ctx context.Context, rc ports.RoutingContext, from, to string, outcome int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := edgeKey{from: from, to: to}
	e := s.edges[k]
	if e == nil {
		e = &portsEdge{}
		s.edges[k] = e
	}
	if outcome == 0 {
		e.Success += 1.0
	} else {
		e.Failure += 1.0
	}
	total := e.Success + e.Failure
	if total > 0 {
		if rc.Cost > 0 {
			e.Cost = (e.Cost*float64(total-1) + rc.Cost) / float64(total)
		}
		if rc.Latency > 0 {
			e.Latency = (e.Latency*float64(total-1) + rc.Latency) / float64(total)
		}
	}
	s.recentTransitions = append(s.recentTransitions, contextTransition{
		Intent: rc.IntentText, From: from, To: to, Outcome: outcome,
	})
	if len(s.recentTransitions) > recentTransitionsCap {
		s.recentTransitions = s.recentTransitions[len(s.recentTransitions)-recentTransitionsCap:]
	}
	return nil
}

func (s *RoutingGraph) RestoreEdges(edges []EdgeStat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.edges = make(map[edgeKey]*portsEdge)
	for _, e := range edges {
		k := edgeKey{from: e.From, to: e.To}
		s.edges[k] = &portsEdge{Success: float64(e.Success), Failure: float64(e.Failure)}
	}
}

func (s *RoutingGraph) RecordTrajectory(path []ports.TransitionStep, score float64, success bool) {
	if len(path) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalRuns++
	for i, step := range path {
		w := math.Pow(trajectoryGamma, float64(len(path)-1-i))
		k := edgeKey{from: step.From, to: step.To}
		e := s.edges[k]
		if e == nil {
			e = &portsEdge{}
			s.edges[k] = e
		}
		if success {
			e.Success += w
		} else {
			e.Failure += w
		}
	}
}

func (s *RoutingGraph) ListEdges() []EdgeStat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]EdgeStat, 0, len(s.edges))
	for k, e := range s.edges {
		out = append(out, EdgeStat{From: k.from, To: k.to, Success: int(e.Success), Failure: int(e.Failure)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

func (s *RoutingGraph) StaticTopology() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := make(map[string][]string, len(s.static))
	for k, v := range s.static {
		m[k] = append([]string(nil), v...)
	}
	return m
}

func intentWords(t string) map[string]struct{} {
	m := make(map[string]struct{})
	f := func(r rune) bool { return unicode.IsSpace(r) || r == ',' || r == '.' }
	for _, w := range strings.FieldsFunc(strings.ToLower(t), f) {
		if len(w) >= 2 {
			m[w] = struct{}{}
		}
	}
	return m
}

func wordOverlap(a, b map[string]struct{}) float64 {
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
