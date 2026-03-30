package model

import (
	"fmt"
)

// TaskRow is one persisted task (JSON payload).
type TaskRow struct {
	ID      string
	Payload string
}

// TaskSelectAll loads every task row.
func (a *AgentDB) TaskSelectAll() ([]TaskRow, error) {
	rows, err := a.DB.Query(fmt.Sprintf(`SELECT id, payload FROM %s`, TableTasks))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskRow
	for rows.Next() {
		var r TaskRow
		if err := rows.Scan(&r.ID, &r.Payload); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// TaskUpsert inserts or replaces the task payload.
func (a *AgentDB) TaskUpsert(id, payload string) error {
	_, err := a.DB.Exec(fmt.Sprintf(`INSERT INTO %s (id, payload) VALUES (?, ?)
			ON CONFLICT(id) DO UPDATE SET payload = excluded.payload`, TableTasks), id, payload)
	return err
}
