package model

import (
	"fmt"
)

// RecallChunkRow is one persisted recall chunk (text + optional embedding blob).
type RecallChunkRow struct {
	Text      string
	Embedding []byte
}

// RecallSelectAllOrdered loads all chunks in id order.
func (a *AgentDB) RecallSelectAllOrdered() ([]RecallChunkRow, error) {
	rows, err := a.DB.Query(fmt.Sprintf(`SELECT text, embedding FROM %s ORDER BY id ASC`, TableRecallChunks))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecallChunkRow
	for rows.Next() {
		var r RecallChunkRow
		if err := rows.Scan(&r.Text, &r.Embedding); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RecallInsert appends one chunk with optional embedding blob (nil for empty).
func (a *AgentDB) RecallInsert(text string, embeddingBlob any) error {
	_, err := a.DB.Exec(fmt.Sprintf(`INSERT INTO %s (text, embedding) VALUES (?, ?)`, TableRecallChunks), text, embeddingBlob)
	return err
}
