package model

import (
	"database/sql"
	"fmt"
)

// SkillPersistRow is one skill row for SQLite sync (payload is JSON of the skill record).
type SkillPersistRow struct {
	Name       string
	SourceFile string
	Payload    string
}

// SkillsReplaceAll replaces the entire skills table with rows in one transaction.
func (a *AgentDB) SkillsReplaceAll(rows []SkillPersistRow) error {
	if a == nil || a.DB == nil {
		return fmt.Errorf("model: agent db is nil")
	}
	tx, err := a.DB.Begin()
	if err != nil {
		return fmt.Errorf("model: skills tx begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := fmt.Sprintf(`DELETE FROM %s`, TableSkills)
	if _, err := tx.Exec(q); err != nil {
		return fmt.Errorf("model: skills delete all: %w", err)
	}

	ins := fmt.Sprintf(`INSERT INTO %s (name, source_file, payload) VALUES (?, ?, ?)`, TableSkills)
	for i := range rows {
		r := rows[i]
		if _, err := tx.Exec(ins, r.Name, r.SourceFile, r.Payload); err != nil {
			return fmt.Errorf("model: skills insert %q: %w", r.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("model: skills tx commit: %w", err)
	}
	return nil
}

// SkillsSelectAll returns every persisted skill payload (JSON), ordered by name.
func (a *AgentDB) SkillsSelectAll() ([]string, error) {
	if a == nil || a.DB == nil {
		return nil, fmt.Errorf("model: agent db is nil")
	}
	q := fmt.Sprintf(`SELECT payload FROM %s ORDER BY name`, TableSkills)
	rows, err := a.DB.Query(q)
	if err != nil {
		return nil, fmt.Errorf("model: skills select all: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		out = append(out, payload)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// SkillsSelectAllRows returns every persisted skill row, ordered by name.
func (a *AgentDB) SkillsSelectAllRows() ([]SkillPersistRow, error) {
	if a == nil || a.DB == nil {
		return nil, fmt.Errorf("model: agent db is nil")
	}
	q := fmt.Sprintf(`SELECT name, source_file, payload FROM %s ORDER BY name`, TableSkills)
	rows, err := a.DB.Query(q)
	if err != nil {
		return nil, fmt.Errorf("model: skills select all rows: %w", err)
	}
	defer rows.Close()

	var out []SkillPersistRow
	for rows.Next() {
		var r SkillPersistRow
		if err := rows.Scan(&r.Name, &r.SourceFile, &r.Payload); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// SkillUpsert inserts or replaces one skill row (primary key is name).
func (a *AgentDB) SkillUpsert(row SkillPersistRow) error {
	if a == nil || a.DB == nil {
		return fmt.Errorf("model: agent db is nil")
	}
	q := fmt.Sprintf(`INSERT INTO %s (name, source_file, payload) VALUES (?, ?, ?)
ON CONFLICT(name) DO UPDATE SET source_file = excluded.source_file, payload = excluded.payload`, TableSkills)
	if _, err := a.DB.Exec(q, row.Name, row.SourceFile, row.Payload); err != nil {
		return fmt.Errorf("model: skills upsert %q: %w", row.Name, err)
	}
	return nil
}

// SkillPayloadBySourceFile returns the JSON payload for a skill originating from source_file (path relative to skills root, slash-separated).
func (a *AgentDB) SkillPayloadBySourceFile(sourceFile string) (string, error) {
	if a == nil || a.DB == nil {
		return "", fmt.Errorf("model: agent db is nil")
	}
	q := fmt.Sprintf(`SELECT payload FROM %s WHERE source_file = ?`, TableSkills)
	var payload string
	err := a.DB.QueryRow(q, sourceFile).Scan(&payload)
	if err == sql.ErrNoRows {
		return "", sql.ErrNoRows
	}
	if err != nil {
		return "", fmt.Errorf("model: skills select by source_file: %w", err)
	}
	return payload, nil
}
