package routinggraph

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"sync"

	"github.com/OctoSucker/agent/internal/runtime/store/capability"
	"github.com/OctoSucker/agent/pkg/mcpclient"
	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
)

const recentTransitionsCap = 200
const trajectoryGamma = 0.9
const globalDistInf = 1e18

// RoutingGraph holds in-memory topology + edge stats and implements routing algorithms (Confidence, Frontier, global pick).
// Optional db enables SQLite persistence; all SQL and write-through live in routing_graph_storage.go.
type RoutingGraph struct {
	mu                sync.RWMutex
	edges             map[edgeKey]*portsEdge
	static            map[string][]string
	recentTransitions []contextTransition
	totalRuns         int64
	db                *sql.DB
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

// NewRoutingGraphFromCapabilityRegistry builds static topology from a capability registry and optionally loads state from SQLite.
func NewRoutingGraphFromCapabilityRegistry(reg *capability.CapabilityRegistry, db *sql.DB) (*RoutingGraph, error) {
	var m map[string]mcpclient.Capability
	if reg != nil {
		m = reg.AllCapabilities()
	}
	return newRoutingGraphFromCapabilityMap(m, db)
}

func newRoutingGraphFromCapabilityMap(m map[string]mcpclient.Capability, db *sql.DB) (*RoutingGraph, error) {
	if m == nil {
		m = map[string]mcpclient.Capability{}
	}
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	static := make(map[string][]string, len(ids)+1)
	static[""] = append([]string(nil), ids...)
	for _, id := range ids {
		static[id] = []string{}
	}
	g := &RoutingGraph{edges: make(map[edgeKey]*portsEdge), static: static, db: db}
	if db != nil {
		if err := g.loadFromDB(); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (s *RoutingGraph) Confidence(ctx context.Context, rc ports.RoutingContext, last string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	next := s.static[last]
	if len(next) == 0 {
		next = s.entryNodesLocked()
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
		next = s.entryNodesLocked()
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

func (s *RoutingGraph) entryNodesLocked() []string {
	return append([]string(nil), s.static[""]...)
}

func (s *RoutingGraph) similarIntentScoreLocked(intent, from, to string) float64 {
	if intent == "" || len(s.recentTransitions) == 0 {
		return 0
	}
	iw := rtutils.RoutingIntentWordSet(intent)
	if len(iw) == 0 {
		return 0
	}
	var success, total int
	for _, t := range s.recentTransitions {
		if t.From != from || t.To != to {
			continue
		}
		tw := rtutils.RoutingIntentWordSet(t.Intent)
		if rtutils.RoutingWordOverlapRatio(iw, tw) < 0.2 {
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

// edgeWeightGlobalLocked returns a non-negative cost for traversing from→to (higher = worse).
// Uses empirical success rate and optional cost/latency on the edge (same signals as Frontier).
func (s *RoutingGraph) edgeWeightGlobalLocked(_ ports.RoutingContext, from, to string) float64 {
	k := edgeKey{from: from, to: to}
	e := s.edges[k]
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

// distToGoalAllLocked runs Dijkstra from goal on the reverse graph: dist[v] = shortest cost from v to goal along forward edges.
func (s *RoutingGraph) distToGoalAllLocked(rc ports.RoutingContext, goal string) map[string]float64 {
	rev := make(map[string][]string)
	for from, tos := range s.static {
		for _, to := range tos {
			rev[to] = append(rev[to], from)
		}
	}
	verts := map[string]struct{}{}
	for from, tos := range s.static {
		verts[from] = struct{}{}
		for _, to := range tos {
			verts[to] = struct{}{}
		}
	}
	if _, ok := verts[goal]; !ok {
		return nil
	}
	dist := make(map[string]float64)
	for v := range verts {
		dist[v] = globalDistInf
	}
	dist[goal] = 0
	visited := make(map[string]bool)
	for {
		var u string
		best := globalDistInf
		for v := range verts {
			if visited[v] {
				continue
			}
			if dist[v] < best {
				best = dist[v]
				u = v
			}
		}
		if best >= globalDistInf {
			break
		}
		visited[u] = true
		for _, pred := range rev[u] {
			w := s.edgeWeightGlobalLocked(rc, pred, u)
			nd := dist[u] + w
			if nd < dist[pred] {
				dist[pred] = nd
			}
		}
	}
	return dist
}

// PickGlobalBestNext returns the feasible candidate c that minimizes edgeWeight(last,c)+distToGoal(c).
func (s *RoutingGraph) PickGlobalBestNext(_ context.Context, rc ports.RoutingContext, last, goal string, candidates []string) (string, bool) {
	if len(candidates) == 0 {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	dist := s.distToGoalAllLocked(rc, goal)
	if dist == nil {
		return "", false
	}
	nextFromLast := map[string]struct{}{}
	for _, to := range s.static[last] {
		nextFromLast[to] = struct{}{}
	}
	bestC := ""
	bestCost := globalDistInf
	for _, c := range candidates {
		if _, ok := nextFromLast[c]; !ok {
			continue
		}
		w := s.edgeWeightGlobalLocked(rc, last, c)
		d := dist[c]
		if d >= globalDistInf-1 {
			continue
		}
		cost := w + d
		if cost < bestCost {
			bestCost = cost
			bestC = c
		}
	}
	if bestC == "" {
		return "", false
	}
	return bestC, true
}

// PickBestByImmediateEdge returns the feasible candidate c with minimal immediate edge weight.
func (s *RoutingGraph) PickBestByImmediateEdge(_ context.Context, rc ports.RoutingContext, last string, candidates []string) (string, bool) {
	if len(candidates) == 0 {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	nextFromLast := map[string]struct{}{}
	for _, to := range s.static[last] {
		nextFromLast[to] = struct{}{}
	}
	bestC := ""
	bestWeight := globalDistInf
	for _, c := range candidates {
		if _, ok := nextFromLast[c]; !ok {
			continue
		}
		w := s.edgeWeightGlobalLocked(rc, last, c)
		if w < bestWeight {
			bestWeight = w
			bestC = c
		}
	}
	if bestC == "" {
		return "", false
	}
	return bestC, true
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
	return s.persistEdgeAndRecentLocked(k, e)
}

func (s *RoutingGraph) RestoreEdges(edges []EdgeStat) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.edges = make(map[edgeKey]*portsEdge)
	for _, e := range edges {
		k := edgeKey{from: e.From, to: e.To}
		s.edges[k] = &portsEdge{Success: float64(e.Success), Failure: float64(e.Failure)}
	}
	return s.persistAllEdgesLocked()
}

func (s *RoutingGraph) RecordTrajectory(path []ports.TransitionStep, score float64, success bool) error {
	if len(path) == 0 {
		return nil
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
	return s.persistTrajectoryLocked(path)
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
