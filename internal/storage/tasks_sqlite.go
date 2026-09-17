package storage

import (
	"fmt"
	"time"
)

// LoadTaskSnapshots returns serialized task snapshots in creation order.
func (d *DB) LoadTaskSnapshots() ([][]byte, error) {
	rows, err := d.conn.Query(fmt.Sprintf(
		`SELECT snapshot_json FROM %s ORDER BY updated_at ASC, id ASC`, TableTaskSnapshots))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		out = append(out, append([]byte(nil), payload...))
	}
	return out, rows.Err()
}

// SaveTaskSnapshot atomically inserts or replaces one serialized snapshot.
func (d *DB) SaveTaskSnapshot(id string, payload []byte) error {
	if d == nil || d.conn == nil {
		return fmt.Errorf("store: database is closed")
	}
	if id == "" || len(payload) == 0 {
		return fmt.Errorf("store: task id and snapshot are required")
	}
	_, err := d.conn.Exec(fmt.Sprintf(`INSERT INTO %s (id, snapshot_json, updated_at) VALUES (?,?,?)
		ON CONFLICT(id) DO UPDATE SET snapshot_json=excluded.snapshot_json, updated_at=excluded.updated_at`, TableTaskSnapshots),
		id, payload, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
