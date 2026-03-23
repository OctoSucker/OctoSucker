package config

import (
	"fmt"
	"os"
	"path/filepath"
)

func ResolveAndEnsureDir(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("set -workspace to the agent workspace directory")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return "", err
	}
	return abs, nil
}

func ConfigFile(root string) string {
	return filepath.Join(root, "config.json")
}
