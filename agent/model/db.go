package model

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// AgentDB is the workspace SQLite handle: schema migration on open and typed persistence methods.
type AgentDB struct {
	DB *sql.DB
}

// OpenAgentDB opens the workspace SQLite file, runs migrations (CREATE IF NOT EXISTS), and returns a store.
func OpenAgentDB(workspaceRoot string) (*AgentDB, error) {
	if workspaceRoot == "" {
		return nil, fmt.Errorf("model: workspace root required for sqlite")
	}
	dataDir := filepath.Join(workspaceRoot, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("model: mkdir data: %w", err)
	}
	path := filepath.Join(dataDir, "octoplus.sqlite")
	dsn := "file:" + filepath.ToSlash(path) + "?_busy_timeout=5000&_journal_mode=WAL"
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("model: open sqlite (gorm): %w", err)
	}
	db, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("model: gorm db handle: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		if cerr := db.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close sqlite after ping failure: %w", cerr))
		}
		return nil, fmt.Errorf("model: ping sqlite: %w", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		if cerr := db.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close sqlite after pragma failure: %w", cerr))
		}
		return nil, fmt.Errorf("model: pragma foreign_keys: %w", err)
	}
	a := &AgentDB{DB: db}
	if err := a.migrate(); err != nil {
		if cerr := db.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close sqlite after migration failure: %w", cerr))
		}
		return nil, err
	}
	return a, nil
}

// Close closes the underlying database.
func (a *AgentDB) Close() error {
	if a == nil || a.DB == nil {
		return nil
	}
	return a.DB.Close()
}

func (a *AgentDB) migrate() error {
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
		if _, err := a.DB.Exec(q); err != nil {
			return fmt.Errorf("model migrate: %w", err)
		}
	}
	if _, err := a.DB.Exec(`DROP TABLE IF EXISTS kg_node_aliases`); err != nil {
		return fmt.Errorf("model migrate: drop legacy kg_node_aliases: %w", err)
	}
	return a.migrateKnowledgeGraphNodeEmbeddingColumn()
}

func (a *AgentDB) migrateKnowledgeGraphNodeEmbeddingColumn() error {
	var cnt int
	q := fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info(%q) WHERE name = 'embedding'`, TableKnowledgeGraphNodes)
	if err := a.DB.QueryRow(q).Scan(&cnt); err != nil {
		return fmt.Errorf("model migrate kg_node embedding pragma: %w", err)
	}
	if cnt > 0 {
		return nil
	}
	if _, err := a.DB.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN embedding BLOB`, TableKnowledgeGraphNodes)); err != nil {
		return fmt.Errorf("model migrate kg_node add embedding: %w", err)
	}
	return nil
}
