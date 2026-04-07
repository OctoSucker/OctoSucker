// Package store holds workspace SQLite: schema migration on open and typed persistence methods.
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

const (
	sqliteFilename       = "octosucker.sqlite"
	legacySQLiteFilename = "octoplus.sqlite"
)

// DB is the workspace SQLite handle.
type DB struct {
	conn *sql.DB
}

// Open opens the workspace SQLite file, runs migrations (CREATE IF NOT EXISTS), and returns a handle.
func Open(workspaceRoot string) (*DB, error) {
	if workspaceRoot == "" {
		return nil, fmt.Errorf("store: workspace root required for sqlite")
	}
	dataDir := filepath.Join(workspaceRoot, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("store: mkdir data: %w", err)
	}
	path, err := resolveSQLitePath(dataDir)
	if err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(path) + "?_busy_timeout=5000&_journal_mode=WAL"
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite (gorm): %w", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("store: gorm db handle: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := sqlDB.Ping(); err != nil {
		if cerr := sqlDB.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close sqlite after ping failure: %w", cerr))
		}
		return nil, fmt.Errorf("store: ping sqlite: %w", err)
	}
	if _, err := sqlDB.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		if cerr := sqlDB.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close sqlite after pragma failure: %w", cerr))
		}
		return nil, fmt.Errorf("store: pragma foreign_keys: %w", err)
	}
	d := &DB{conn: sqlDB}
	if err := d.migrate(); err != nil {
		if cerr := sqlDB.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close sqlite after migration failure: %w", cerr))
		}
		return nil, err
	}
	return d, nil
}

func resolveSQLitePath(dataDir string) (string, error) {
	current := filepath.Join(dataDir, sqliteFilename)
	_, err := os.Stat(current)
	if err == nil {
		return current, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("store: stat sqlite: %w", err)
	}
	legacy := filepath.Join(dataDir, legacySQLiteFilename)
	if _, err := os.Stat(legacy); err != nil {
		if os.IsNotExist(err) {
			return current, nil
		}
		return "", fmt.Errorf("store: stat legacy sqlite: %w", err)
	}
	if err := os.Rename(legacy, current); err != nil {
		return "", fmt.Errorf("store: rename %s → %s: %w", legacySQLiteFilename, sqliteFilename, err)
	}
	return current, nil
}

// Close closes the underlying database.
func (d *DB) Close() error {
	if d == nil || d.conn == nil {
		return nil
	}
	return d.conn.Close()
}

func (d *DB) migrate() error {
	stmts := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			from_tool TEXT NOT NULL,
			to_tool TEXT NOT NULL,
			success REAL NOT NULL DEFAULT 0,
			failure REAL NOT NULL DEFAULT 0,
			cost REAL NOT NULL DEFAULT 0,
			latency REAL NOT NULL DEFAULT 0,
			PRIMARY KEY (from_tool, to_tool)
		)`, TableRoutingEdges),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			intent TEXT NOT NULL,
			from_tool TEXT NOT NULL,
			to_tool TEXT NOT NULL,
			outcome INTEGER NOT NULL
		)`, TableRoutingTransitions),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id TEXT NOT NULL PRIMARY KEY,
			embedding BLOB
		)`, TableKnowledgeGraphNodes),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			from_id TEXT NOT NULL,
			to_id TEXT NOT NULL,
			positive INTEGER NOT NULL,
			PRIMARY KEY (from_id, to_id),
			FOREIGN KEY (from_id) REFERENCES %s(id),
			FOREIGN KEY (to_id) REFERENCES %s(id)
		)`, TableKnowledgeGraphEdges, TableKnowledgeGraphNodes, TableKnowledgeGraphNodes),
	}
	for _, q := range stmts {
		if _, err := d.conn.Exec(q); err != nil {
			return fmt.Errorf("store migrate: %w", err)
		}
	}
	if _, err := d.conn.Exec(`DROP TABLE IF EXISTS tasks`); err != nil {
		return fmt.Errorf("store migrate: drop tasks (in-memory task store only): %w", err)
	}
	if _, err := d.conn.Exec(`DROP TABLE IF EXISTS kg_node_aliases`); err != nil {
		return fmt.Errorf("store migrate: drop legacy kg_node_aliases: %w", err)
	}
	if err := d.migrateRoutingEdgesToolColumns(); err != nil {
		return err
	}
	return d.migrateKnowledgeGraphNodeEmbeddingColumn()
}

func (d *DB) migrateRoutingEdgesToolColumns() error {
	var oldFrom, newFrom int
	qOld := fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info(%q) WHERE name = 'from_cap'`, TableRoutingEdges)
	if err := d.conn.QueryRow(qOld).Scan(&oldFrom); err != nil {
		return fmt.Errorf("store migrate routing_edges pragma from_cap: %w", err)
	}
	qNew := fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info(%q) WHERE name = 'from_tool'`, TableRoutingEdges)
	if err := d.conn.QueryRow(qNew).Scan(&newFrom); err != nil {
		return fmt.Errorf("store migrate routing_edges pragma from_tool: %w", err)
	}
	if oldFrom == 0 || newFrom > 0 {
		return nil
	}
	if _, err := d.conn.Exec(fmt.Sprintf(`ALTER TABLE %s RENAME COLUMN from_cap TO from_tool`, TableRoutingEdges)); err != nil {
		return fmt.Errorf("store migrate routing_edges rename from_cap: %w", err)
	}
	if _, err := d.conn.Exec(fmt.Sprintf(`ALTER TABLE %s RENAME COLUMN to_cap TO to_tool`, TableRoutingEdges)); err != nil {
		return fmt.Errorf("store migrate routing_edges rename to_cap: %w", err)
	}
	return nil
}

func (d *DB) migrateKnowledgeGraphNodeEmbeddingColumn() error {
	var cnt int
	q := fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info(%q) WHERE name = 'embedding'`, TableKnowledgeGraphNodes)
	if err := d.conn.QueryRow(q).Scan(&cnt); err != nil {
		return fmt.Errorf("store migrate kg_node embedding pragma: %w", err)
	}
	if cnt > 0 {
		return nil
	}
	if _, err := d.conn.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN embedding BLOB`, TableKnowledgeGraphNodes)); err != nil {
		return fmt.Errorf("store migrate kg_node add embedding: %w", err)
	}
	return nil
}
