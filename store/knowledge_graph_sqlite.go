package store

import (
	"database/sql"
	"fmt"
)

// KnowledgeGraphEdgeRow is one persisted influence edge (from_id → to_id, correlation sign).
type KnowledgeGraphEdgeRow struct {
	FromID   string `json:"from_id"`
	ToID     string `json:"to_id"`
	Positive bool   `json:"positive"`
}

// KnowledgeGraphNodeExists reports whether a node id is stored.
func (d *DB) KnowledgeGraphNodeExists(id string) (bool, error) {
	var n int
	err := d.conn.QueryRow(fmt.Sprintf(`SELECT 1 FROM %s WHERE id = ? LIMIT 1`, TableKnowledgeGraphNodes), id).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// KnowledgeGraphNodeRow is one knowledge-graph node row (optional embedding blob).
type KnowledgeGraphNodeRow struct {
	ID        string
	Embedding []byte
}

// KnowledgeGraphNodeInsert inserts a node row. embedding may be nil. It fails if id is empty or the id already exists.
func (d *DB) KnowledgeGraphNodeInsert(id string, embedding []byte) error {
	if id == "" {
		return fmt.Errorf("store: KnowledgeGraphNodeInsert: empty id")
	}
	_, err := d.conn.Exec(fmt.Sprintf(`INSERT INTO %s (id, embedding) VALUES (?, ?)`, TableKnowledgeGraphNodes), id, embedding)
	return err
}

// KnowledgeGraphNodesSelectAll returns every node row in arbitrary order.
func (d *DB) KnowledgeGraphNodesSelectAll() ([]KnowledgeGraphNodeRow, error) {
	rows, err := d.conn.Query(fmt.Sprintf(`SELECT id, embedding FROM %s`, TableKnowledgeGraphNodes))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KnowledgeGraphNodeRow
	for rows.Next() {
		var r KnowledgeGraphNodeRow
		if err := rows.Scan(&r.ID, &r.Embedding); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// KnowledgeGraphEdgeExists reports whether a directed edge from → to exists.
func (d *DB) KnowledgeGraphEdgeExists(from, to string) (bool, error) {
	var n int
	err := d.conn.QueryRow(fmt.Sprintf(
		`SELECT 1 FROM %s WHERE from_id = ? AND to_id = ? LIMIT 1`,
		TableKnowledgeGraphEdges), from, to).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// KnowledgeGraphEdgeSelect loads one edge by endpoints. ok is false when no row exists.
func (d *DB) KnowledgeGraphEdgeSelect(from, to string) (KnowledgeGraphEdgeRow, bool, error) {
	var pos int
	err := d.conn.QueryRow(fmt.Sprintf(
		`SELECT positive FROM %s WHERE from_id = ? AND to_id = ?`,
		TableKnowledgeGraphEdges), from, to).Scan(&pos)
	if err == sql.ErrNoRows {
		return KnowledgeGraphEdgeRow{}, false, nil
	}
	if err != nil {
		return KnowledgeGraphEdgeRow{}, false, err
	}
	return KnowledgeGraphEdgeRow{FromID: from, ToID: to, Positive: pos != 0}, true, nil
}

// KnowledgeGraphEdgeInsert inserts an edge row. Endpoints must reference existing nodes.
func (d *DB) KnowledgeGraphEdgeInsert(from, to string, positive bool) error {
	if from == "" || to == "" {
		return fmt.Errorf("store: KnowledgeGraphEdgeInsert: empty from or to")
	}
	p := 0
	if positive {
		p = 1
	}
	_, err := d.conn.Exec(fmt.Sprintf(
		`INSERT INTO %s (from_id, to_id, positive) VALUES (?, ?, ?)`,
		TableKnowledgeGraphEdges), from, to, p)
	return err
}

// KnowledgeGraphEdgesSelectAll returns every edge in arbitrary order.
func (d *DB) KnowledgeGraphEdgesSelectAll() ([]KnowledgeGraphEdgeRow, error) {
	rows, err := d.conn.Query(fmt.Sprintf(
		`SELECT from_id, to_id, positive FROM %s`, TableKnowledgeGraphEdges))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KnowledgeGraphEdgeRow
	for rows.Next() {
		var fromID, toID string
		var pos int
		if err := rows.Scan(&fromID, &toID, &pos); err != nil {
			return nil, err
		}
		out = append(out, KnowledgeGraphEdgeRow{FromID: fromID, ToID: toID, Positive: pos != 0})
	}
	return out, rows.Err()
}
