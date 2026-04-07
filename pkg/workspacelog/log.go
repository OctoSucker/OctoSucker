// Package workspacelog opens append-only log files under <workspace>/logs/.
package workspacelog

import (
	"fmt"
	"os"
	"path/filepath"
)

// OpenFile ensures workspaceRoot/logs exists and opens workspaceRoot/logs/baseName for append.
// baseName must be a single path segment (e.g. "agent.log"), not a relative path.
func OpenFile(workspaceRoot, baseName string) (*os.File, string, error) {
	if workspaceRoot == "" {
		return nil, "", fmt.Errorf("workspacelog: empty workspace root")
	}
	if baseName == "" || filepath.Base(baseName) != baseName {
		return nil, "", fmt.Errorf("workspacelog: baseName must be a non-empty file name without path separators")
	}
	logDir := filepath.Join(workspaceRoot, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, "", fmt.Errorf("workspacelog: mkdir logs: %w", err)
	}
	logPath := filepath.Join(logDir, baseName)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, logPath, fmt.Errorf("workspacelog: open %q: %w", logPath, err)
	}
	return f, logPath, nil
}
