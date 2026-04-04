package model

import (
	"database/sql"
	"fmt"
	"strconv"
)

// RoutingEdgeRow is one row in routing_edges.
type RoutingEdgeRow struct {
	FromTool string
	ToTool   string
	Success float64
	Failure float64
	Cost    float64
	Latency float64
}

// RoutingEdgesSelectAll loads every edge row.
func (a *AgentDB) RoutingEdgesSelectAll() ([]RoutingEdgeRow, error) {
	rows, err := a.DB.Query(fmt.Sprintf(`SELECT from_tool, to_tool, success, failure, cost, latency FROM %s`, TableRoutingEdges))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RoutingEdgeRow
	for rows.Next() {
		var r RoutingEdgeRow
		if err := rows.Scan(&r.FromTool, &r.ToTool, &r.Success, &r.Failure, &r.Cost, &r.Latency); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RoutingMetaGet returns (value, true) when the key exists and is non-null.
func (a *AgentDB) RoutingMetaGet(key string) (string, bool, error) {
	var v sql.NullString
	err := a.DB.QueryRow(fmt.Sprintf(`SELECT v FROM %s WHERE k = ?`, TableRoutingMeta), key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !v.Valid {
		return "", false, nil
	}
	return v.String, true, nil
}

// RoutingMetaUpsert sets a key in routing_meta.
func (a *AgentDB) RoutingMetaUpsert(key, value string) error {
	_, err := a.DB.Exec(fmt.Sprintf(`INSERT INTO %s (k, v) VALUES (?, ?)
		ON CONFLICT(k) DO UPDATE SET v = excluded.v`, TableRoutingMeta), key, value)
	return err
}

// RoutingEdgeUpsert inserts or updates one edge.
func (a *AgentDB) RoutingEdgeUpsert(r RoutingEdgeRow) error {
	_, err := a.DB.Exec(fmt.Sprintf(`INSERT INTO %s (from_tool, to_tool, success, failure, cost, latency) VALUES (?,?,?,?,?,?)
		ON CONFLICT(from_tool, to_tool) DO UPDATE SET
			success = excluded.success,
			failure = excluded.failure,
			cost = excluded.cost,
			latency = excluded.latency`, TableRoutingEdges),
		r.FromTool, r.ToTool, r.Success, r.Failure, r.Cost, r.Latency)
	return err
}

// RoutingTotalRunsMetaKey is the routing_meta key for total run count.
const RoutingTotalRunsMetaKey = "total_runs"

// RoutingRecentTransitionsMetaKey is the routing_meta key for recent transitions JSON.
const RoutingRecentTransitionsMetaKey = "recent_transitions"

// RoutingSaveTotalRuns persists total_runs as a decimal string.
func (a *AgentDB) RoutingSaveTotalRuns(n int64) error {
	return a.RoutingMetaUpsert(RoutingTotalRunsMetaKey, strconv.FormatInt(n, 10))
}
