package store

import "fmt"

const routingTransitionsMaxRows = 200

// EdgeKey identifies a directed edge: canonical tool ids (Node.String() in routegraph), "" for entry → tool.
type EdgeKey struct {
	From string
	To   string
}

// ContextTransition is one routing_transitions row / in-memory recent (intent, from→to) observation.
type ContextTransition struct {
	Intent  string `json:"intent"`
	From    string `json:"from"`
	To      string `json:"to"`
	Outcome bool   `json:"outcome"`
}

// RoutingEdgeRow is one routing_edges row and the in-memory per-edge stats shape used by routegraph.Graph.
type RoutingEdgeRow struct {
	FromTool string
	ToTool   string
	Success  float64
	Failure  float64
	Cost     float64
	Latency  float64
}

// RoutingEdgesSelectAll loads every edge row keyed by (from_tool, to_tool).
func (d *DB) RoutingEdgesSelectAll() (map[EdgeKey]*RoutingEdgeRow, error) {
	rows, err := d.conn.Query(fmt.Sprintf(`SELECT from_tool, to_tool, success, failure, cost, latency FROM %s`, TableRoutingEdges))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []RoutingEdgeRow
	for rows.Next() {
		var r RoutingEdgeRow
		if err := rows.Scan(&r.FromTool, &r.ToTool, &r.Success, &r.Failure, &r.Cost, &r.Latency); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make(map[EdgeKey]*RoutingEdgeRow, len(list))
	for i := range list {
		r := &list[i]
		k := EdgeKey{From: r.FromTool, To: r.ToTool}
		out[k] = r
	}
	return out, nil
}

// RoutingTransitionsSelectAll returns all rows in chronological order (by id).
func (d *DB) RoutingTransitionsSelectAll() ([]ContextTransition, error) {
	rows, err := d.conn.Query(fmt.Sprintf(
		`SELECT intent, from_tool, to_tool, outcome FROM %s ORDER BY id ASC`, TableRoutingTransitions))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ContextTransition
	for rows.Next() {
		var r ContextTransition
		var o int64
		if err := rows.Scan(&r.Intent, &r.From, &r.To, &o); err != nil {
			return nil, err
		}
		r.Outcome = o != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// RoutingTransitionAppend inserts one transition and deletes oldest rows if count exceeds routingTransitionsMaxRows.
func (d *DB) RoutingTransitionAppend(intent, fromTool, toTool string, outcome bool) error {
	ox := int64(0)
	if outcome {
		ox = 1
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(fmt.Sprintf(
		`INSERT INTO %s (intent, from_tool, to_tool, outcome) VALUES (?,?,?,?)`,
		TableRoutingTransitions), intent, fromTool, toTool, ox); err != nil {
		return err
	}
	var cnt int64
	if err := tx.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, TableRoutingTransitions)).Scan(&cnt); err != nil {
		return err
	}
	if cnt > routingTransitionsMaxRows {
		nDel := cnt - routingTransitionsMaxRows
		if _, err := tx.Exec(fmt.Sprintf(
			`DELETE FROM %s WHERE id IN (SELECT id FROM %s ORDER BY id ASC LIMIT ?)`,
			TableRoutingTransitions, TableRoutingTransitions), nDel); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RoutingEdgeUpsert inserts or updates one edge.
func (d *DB) RoutingEdgeUpsert(r RoutingEdgeRow) error {
	_, err := d.conn.Exec(fmt.Sprintf(`INSERT INTO %s (from_tool, to_tool, success, failure, cost, latency) VALUES (?,?,?,?,?,?)
		ON CONFLICT(from_tool, to_tool) DO UPDATE SET
			success = excluded.success,
			failure = excluded.failure,
			cost = excluded.cost,
			latency = excluded.latency`, TableRoutingEdges),
		r.FromTool, r.ToTool, r.Success, r.Failure, r.Cost, r.Latency)
	return err
}
