package graph

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/OctoSucker/agent/model"
)

func routingEdgeRow(k Key, e *EdgeStat) model.RoutingEdgeRow {
	return model.RoutingEdgeRow{
		FromCap: k.From.String(), ToCap: k.To.String(),
		Success: e.Success, Failure: e.Failure, Cost: e.Cost, Latency: e.Latency,
	}
}

// LoadFromDB reads routing_edges and routing_meta and merges them into g (static topology unchanged).
func (g *Graph) LoadFromDB() error {
	edgeRows, err := g.db.RoutingEdgesSelectAll()
	if err != nil {
		return err
	}
	edges := make(map[Key]*EdgeStat, len(edgeRows))
	for _, r := range edgeRows {
		fromN, ok1 := ParseNode(r.FromCap)
		toN, ok2 := ParseNode(r.ToCap)
		if !ok1 || !ok2 || !toN.IsValid() {
			return fmt.Errorf("graph: routing edge invalid from %q to %q", r.FromCap, r.ToCap)
		}
		k := Key{From: fromN, To: toN}
		edges[k] = &EdgeStat{Success: r.Success, Failure: r.Failure, Cost: r.Cost, Latency: r.Latency}
	}

	var totalRuns int64
	if v, ok, err := g.db.RoutingMetaGet(model.RoutingTotalRunsMetaKey); err != nil {
		return err
	} else if ok && v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("graph: routing total_runs %q: %w", v, err)
		}
		totalRuns = n
	}

	var recent []ContextTransition
	if v, ok, err := g.db.RoutingMetaGet(model.RoutingRecentTransitionsMetaKey); err != nil {
		return err
	} else if ok && v != "" {
		recent, err = decodeRecentTransitionsJSON(v)
		if err != nil {
			return err
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.loadStateLocked(edges, totalRuns, recent)
	return nil
}

func decodeRecentTransitionsJSON(v string) ([]ContextTransition, error) {
	var out []ContextTransition
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return nil, fmt.Errorf("graph: routing recent_transitions JSON: %w", err)
	}
	return out, nil
}

// UpsertRoutingEdge writes one edge row derived from k and e.
func (g *Graph) UpsertRoutingEdge(k Key, e *EdgeStat) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.upsertRoutingEdgeLocked(k, e)
}

func (g *Graph) upsertRoutingEdgeLocked(k Key, e *EdgeStat) error {
	return g.db.RoutingEdgeUpsert(routingEdgeRow(k, e))
}

// PersistRoutingTransition upserts the edge and replaces recent_transitions JSON in meta.
func (g *Graph) PersistRoutingTransition(k Key, e *EdgeStat, recent []ContextTransition) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.persistRoutingTransitionLocked(k, e, recent)
}

func (g *Graph) persistRoutingTransitionLocked(k Key, e *EdgeStat, recent []ContextTransition) error {
	if err := g.upsertRoutingEdgeLocked(k, e); err != nil {
		return err
	}
	b, err := json.Marshal(recent)
	if err != nil {
		return err
	}
	return g.db.RoutingMetaUpsert(model.RoutingRecentTransitionsMetaKey, string(b))
}

// PersistRoutingTotalRuns stores the trajectory counter in routing_meta.
func (g *Graph) PersistRoutingTotalRuns(n int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.persistRoutingTotalRunsLocked(n)
}

func (g *Graph) persistRoutingTotalRunsLocked(n int64) error {
	return g.db.RoutingSaveTotalRuns(n)
}
