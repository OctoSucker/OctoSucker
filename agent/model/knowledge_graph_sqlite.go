package model

import (
	"database/sql"
	"fmt"
)

// KnowledgeGraphEdgeRow is one persisted influence edge (from → to, correlation sign).
type KnowledgeGraphEdgeRow struct {
	FromID   string
	ToID     string
	Positive bool
}

// KnowledgeGraphNodeExists reports whether a node id is stored.
func (a *AgentDB) KnowledgeGraphNodeExists(id string) (bool, error) {
	var n int
	err := a.DB.QueryRow(fmt.Sprintf(`SELECT 1 FROM %s WHERE id = ? LIMIT 1`, TableKnowledgeGraphNodes), id).Scan(&n)
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
func (a *AgentDB) KnowledgeGraphNodeInsert(id string, embedding []byte) error {
	if id == "" {
		return fmt.Errorf("model: KnowledgeGraphNodeInsert: empty id")
	}
	_, err := a.DB.Exec(fmt.Sprintf(`INSERT INTO %s (id, embedding) VALUES (?, ?)`, TableKnowledgeGraphNodes), id, embedding)
	return err
}

// KnowledgeGraphNodesSelectAll returns every node row in arbitrary order.
func (a *AgentDB) KnowledgeGraphNodesSelectAll() ([]KnowledgeGraphNodeRow, error) {
	rows, err := a.DB.Query(fmt.Sprintf(`SELECT id, embedding FROM %s`, TableKnowledgeGraphNodes))
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

// KnowledgeGraphEdgeExists reports whether a directed edge from_id → to_id exists.
func (a *AgentDB) KnowledgeGraphEdgeExists(fromID, toID string) (bool, error) {
	var n int
	err := a.DB.QueryRow(fmt.Sprintf(
		`SELECT 1 FROM %s WHERE from_id = ? AND to_id = ? LIMIT 1`,
		TableKnowledgeGraphEdges), fromID, toID).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// KnowledgeGraphEdgeSelect loads one edge by endpoints. ok is false when no row exists.
func (a *AgentDB) KnowledgeGraphEdgeSelect(fromID, toID string) (KnowledgeGraphEdgeRow, bool, error) {
	var pos int
	err := a.DB.QueryRow(fmt.Sprintf(
		`SELECT positive FROM %s WHERE from_id = ? AND to_id = ?`,
		TableKnowledgeGraphEdges), fromID, toID).Scan(&pos)
	if err == sql.ErrNoRows {
		return KnowledgeGraphEdgeRow{}, false, nil
	}
	if err != nil {
		return KnowledgeGraphEdgeRow{}, false, err
	}
	return KnowledgeGraphEdgeRow{FromID: fromID, ToID: toID, Positive: pos != 0}, true, nil
}

// KnowledgeGraphEdgeInsert inserts an edge row. Endpoints must reference existing nodes.
func (a *AgentDB) KnowledgeGraphEdgeInsert(fromID, toID string, positive bool) error {
	if fromID == "" || toID == "" {
		return fmt.Errorf("model: KnowledgeGraphEdgeInsert: empty from or to")
	}
	p := 0
	if positive {
		p = 1
	}
	_, err := a.DB.Exec(fmt.Sprintf(
		`INSERT INTO %s (from_id, to_id, positive) VALUES (?, ?, ?)`,
		TableKnowledgeGraphEdges), fromID, toID, p)
	return err
}

// KnowledgeGraphEdgesSelectAll returns every edge in arbitrary order.
func (a *AgentDB) KnowledgeGraphEdgesSelectAll() ([]KnowledgeGraphEdgeRow, error) {
	rows, err := a.DB.Query(fmt.Sprintf(
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
