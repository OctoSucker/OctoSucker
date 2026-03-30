package graph

import (
	"fmt"
	"sync"

	"github.com/OctoSucker/agent/model"
)

const RecentTransitionsCap = 200

// Graph holds static topology, learned edge weights, recent transitions, and SQLite backing via db.
// A single RWMutex serializes access; exported methods are safe for concurrent use.
// Routing heuristics (confidence, frontier, etc.) live in policy.go in this package.
type Graph struct {
	mu                sync.RWMutex
	edges             map[Key]*EdgeStat
	static            map[Node][]Node
	recentTransitions []ContextTransition
	totalRuns         int64
	db                *model.AgentDB
}

// New returns an empty learned edge map over the given static adjacency bound to db for load/persist.
func New(static map[Node][]Node, db *model.AgentDB) (*Graph, error) {
	if db == nil {
		return nil, fmt.Errorf("graph: AgentDB is nil")
	}
	return &Graph{
		edges:  make(map[Key]*EdgeStat),
		static: cloneStatic(static),
		db:     db,
	}, nil
}

func cloneStatic(m map[Node][]Node) map[Node][]Node {
	if m == nil {
		return map[Node][]Node{}
	}
	out := make(map[Node][]Node, len(m))
	for k, v := range m {
		out[k] = append([]Node(nil), v...)
	}
	return out
}

// ReplaceStatic replaces the static adjacency map (e.g. after capability resync).
func (g *Graph) ReplaceStatic(static map[Node][]Node) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.static = cloneStatic(static)
}

// LoadState merges edge stats, total runs, and recent transitions into g.
// Static topology is not modified.
func (g *Graph) LoadState(edges map[Key]*EdgeStat, totalRuns int64, recent []ContextTransition) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.loadStateLocked(edges, totalRuns, recent)
}

func (g *Graph) loadStateLocked(edges map[Key]*EdgeStat, totalRuns int64, recent []ContextTransition) {
	for k, w := range edges {
		if w == nil {
			continue
		}
		cp := *w
		g.edges[k] = &cp
	}
	g.totalRuns = totalRuns
	g.recentTransitions = append([]ContextTransition(nil), recent...)
}

// TotalRuns returns the trajectory counter used for exploration decay.
func (g *Graph) TotalRuns() int64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.totalRuns
}

// IncTotalRuns increments the trajectory run counter (in-memory only).
func (g *Graph) IncTotalRuns() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.totalRuns++
}

// IncTotalRunsAndPersist increments the trajectory counter and writes routing_meta (exploration decay).
func (g *Graph) IncTotalRunsAndPersist() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.totalRuns++
	return g.persistRoutingTotalRunsLocked(g.totalRuns)
}

// RecentTransitionsClone returns a copy of recent transitions for persistence or scoring.
func (g *Graph) RecentTransitionsClone() []ContextTransition {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.recentTransitionsCloneLocked()
}

func (g *Graph) recentTransitionsCloneLocked() []ContextTransition {
	return append([]ContextTransition(nil), g.recentTransitions...)
}

// Edge returns the mutable edge stats for k, or nil if missing.
func (g *Graph) Edge(k Key) *EdgeStat {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.edges[k]
}

// EnsureEdge returns the edge stat for k, creating an empty one if needed.
func (g *Graph) EnsureEdge(k Key) *EdgeStat {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.ensureEdgeLocked(k)
}

func (g *Graph) ensureEdgeLocked(k Key) *EdgeStat {
	e := g.edges[k]
	if e == nil {
		e = &EdgeStat{}
		g.edges[k] = e
	}
	return e
}

// AddEdgeOutcomeMass adds weighted success/failure mass to k (creates the edge if missing).
func (g *Graph) AddEdgeOutcomeMass(k Key, successDelta, failureDelta float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	e := g.ensureEdgeLocked(k)
	e.Success += successDelta
	e.Failure += failureDelta
}

// AppendRecentTransition appends one observation and enforces RecentTransitionsCap.
func (g *Graph) AppendRecentTransition(ct ContextTransition) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.appendRecentTransitionLocked(ct)
}

func (g *Graph) appendRecentTransitionLocked(ct ContextTransition) {
	g.recentTransitions = append(g.recentTransitions, ct)
	if len(g.recentTransitions) > RecentTransitionsCap {
		g.recentTransitions = g.recentTransitions[len(g.recentTransitions)-RecentTransitionsCap:]
	}
}

// TotalVisits sums success+failure counts across all edges (for exploration heuristics).
func (g *Graph) TotalVisits() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var n int
	for _, e := range g.edges {
		n += int(e.Success + e.Failure)
	}
	return n
}

// EntryNodes returns a copy of successors of the synthetic entry vertex.
func (g *Graph) EntryNodes() []Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return append([]Node(nil), g.static[Node{}]...)
}

// StaticSuccessors returns static[last], or entry nodes when last has no outgoing edges.
func (g *Graph) StaticSuccessors(last Node) []Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.staticSuccessorsLocked(last)
}

func (g *Graph) staticSuccessorsLocked(last Node) []Node {
	next := g.static[last]
	if len(next) == 0 {
		return append([]Node(nil), g.static[Node{}]...)
	}
	return append([]Node(nil), next...)
}
