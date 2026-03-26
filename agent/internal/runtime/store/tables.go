package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// SQLite table names (single source of truth for migrations and queries).
const (
	TableTasks                  = "tasks"
	TableRoutingEdges           = "routing_edges"
	TableRoutingMeta            = "routing_meta"
	TableRecallChunks           = "recall_chunks"
	TableProcedures             = "procedures"
	TableProcedureVariants      = "procedure_variants"
	TableNodeFailureStats       = "node_failure_stats"
	TableProcedureLearnProgress = "procedure_learn_progress"
)

// DefaultSQLiteRelPath is created under the workspace root: data/octoplus.sqlite
const DefaultSQLiteRelPath = "data/octoplus.sqlite"

// OpenAgentDB opens the workspace SQLite file and ensures tables match the current schema (CREATE IF NOT EXISTS only; no old-DB upgrade path).
func OpenAgentDB(workspaceRoot string) (*sql.DB, error) {
	if workspaceRoot == "" {
		return nil, fmt.Errorf("store: workspace root required for sqlite")
	}
	dataDir := filepath.Join(workspaceRoot, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("store: mkdir data: %w", err)
	}
	path := filepath.Join(dataDir, "octoplus.sqlite")
	dsn := "file:" + filepath.ToSlash(path) + "?_busy_timeout=5000&_journal_mode=WAL"
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite (gorm): %w", err)
	}
	db, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("store: gorm db handle: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite: avoid writer lock contention across pools
	if err := db.Ping(); err != nil {
		if cerr := db.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close sqlite after ping failure: %w", cerr))
		}
		return nil, fmt.Errorf("store: ping sqlite: %w", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		if cerr := db.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close sqlite after pragma failure: %w", cerr))
		}
		return nil, fmt.Errorf("store: pragma foreign_keys: %w", err)
	}
	if err := migrateAgentDB(db); err != nil {
		if cerr := db.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close sqlite after migration failure: %w", cerr))
		}
		return nil, err
	}
	return db, nil
}

// migrateAgentDB creates missing tables for the current schema. Schema changes are not applied to existing DB files; delete the sqlite file to reset.
func migrateAgentDB(db *sql.DB) error {
	stmts := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			payload TEXT NOT NULL
		)`, TableTasks),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			from_cap TEXT NOT NULL,
			to_cap TEXT NOT NULL,
			success REAL NOT NULL DEFAULT 0,
			failure REAL NOT NULL DEFAULT 0,
			cost REAL NOT NULL DEFAULT 0,
			latency REAL NOT NULL DEFAULT 0,
			PRIMARY KEY (from_cap, to_cap)
		)`, TableRoutingEdges),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			k TEXT PRIMARY KEY,
			v TEXT NOT NULL
		)`, TableRoutingMeta),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			text TEXT NOT NULL,
			embedding BLOB
		)`, TableRecallChunks),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			name TEXT PRIMARY KEY,
			keywords_json TEXT NOT NULL,
			caps_json TEXT NOT NULL,
			path_json TEXT NOT NULL,
			embedding BLOB,
			attempts INTEGER NOT NULL DEFAULT 0,
			successes INTEGER NOT NULL DEFAULT 0,
			last_used_unix INTEGER NOT NULL DEFAULT 0
		)`, TableProcedures),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			procedure_name TEXT NOT NULL,
			variant_id TEXT NOT NULL,
			plan_json TEXT NOT NULL,
			params_json TEXT NOT NULL DEFAULT '[]',
			attempts INTEGER NOT NULL DEFAULT 0,
			successes INTEGER NOT NULL DEFAULT 0,
			last_used_unix INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (procedure_name, variant_id),
			FOREIGN KEY (procedure_name) REFERENCES %s(name) ON DELETE CASCADE
		)`, TableProcedureVariants, TableProcedures),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			dedup_key TEXT PRIMARY KEY,
			capability TEXT NOT NULL,
			tool TEXT NOT NULL,
			from_cap TEXT NOT NULL DEFAULT '',
			error_sig TEXT NOT NULL,
			failure_count INTEGER NOT NULL DEFAULT 1,
			last_seen_unix INTEGER NOT NULL DEFAULT 0
		)`, TableNodeFailureStats),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_node_failure_cap ON %s (capability)`, TableNodeFailureStats),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_node_failure_from ON %s (from_cap)`, TableNodeFailureStats),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			cap_key TEXT PRIMARY KEY,
			success_count INTEGER NOT NULL DEFAULT 0,
			last_success_unix INTEGER NOT NULL DEFAULT 0
		)`, TableProcedureLearnProgress),
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("store migrate: %w", err)
		}
	}
	return nil
}
