package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// DefaultSQLiteRelPath is created under the workspace root: data/octoplus.sqlite
const DefaultSQLiteRelPath = "data/octoplus.sqlite"

// OpenAgentDB opens (and migrates) the workspace SQLite file under workspaceRoot/data/.
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
		_ = db.Close()
		return nil, fmt.Errorf("store: ping sqlite: %w", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: pragma foreign_keys: %w", err)
	}
	if err := migrateAgentDB(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
