package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveWorkspaceDir resolves the agent home directory and requires it to exist.
// The agent home contains config.json and agent-owned state such as data/, logs/,
// skills/, and knowledge/.
func ResolveWorkspaceDir(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("set -workspace to the agent workspace directory")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("workspace directory %q does not exist", abs)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace path %q is not a directory", abs)
	}
	return abs, nil
}

func ConfigFile(root string) string {
	return filepath.Join(root, "config.json")
}
