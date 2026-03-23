package store

import (
	"database/sql"
	"fmt"
)

const agentDBVersion = 1

func migrateAgentDB(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_meta (
			k TEXT PRIMARY KEY,
			v TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			payload TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS routing_edges (
			from_cap TEXT NOT NULL,
			to_cap TEXT NOT NULL,
			success REAL NOT NULL DEFAULT 0,
			failure REAL NOT NULL DEFAULT 0,
			cost REAL NOT NULL DEFAULT 0,
			latency REAL NOT NULL DEFAULT 0,
			PRIMARY KEY (from_cap, to_cap)
		)`,
		`CREATE TABLE IF NOT EXISTS routing_meta (
			k TEXT PRIMARY KEY,
			v TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS recall_chunks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			text TEXT NOT NULL,
			embedding BLOB
		)`,
		`CREATE TABLE IF NOT EXISTS skills (
			name TEXT PRIMARY KEY,
			keywords_json TEXT NOT NULL,
			caps_json TEXT NOT NULL,
			path_json TEXT NOT NULL,
			embedding BLOB,
			attempts INTEGER NOT NULL DEFAULT 0,
			successes INTEGER NOT NULL DEFAULT 0,
			last_used_unix INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS skill_variants (
			skill_name TEXT NOT NULL,
			variant_id TEXT NOT NULL,
			plan_json TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			successes INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (skill_name, variant_id),
			FOREIGN KEY (skill_name) REFERENCES skills(name) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS capabilities (
			id TEXT PRIMARY KEY,
			tools_json TEXT NOT NULL
		)`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("store migrate: %w", err)
		}
	}
	var ver string
	err := db.QueryRow(`SELECT v FROM schema_meta WHERE k = 'version'`).Scan(&ver)
	if err == sql.ErrNoRows {
		if _, err := db.Exec(`INSERT INTO schema_meta (k, v) VALUES ('version', ?)`, fmt.Sprint(agentDBVersion)); err != nil {
			return fmt.Errorf("store migrate: set version: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("store migrate: read version: %w", err)
	}
	if ver != fmt.Sprint(agentDBVersion) {
		return fmt.Errorf("store migrate: unsupported db version %q (want %d)", ver, agentDBVersion)
	}
	return nil
}
