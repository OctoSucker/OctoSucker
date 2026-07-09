package projectcontext

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxContextRunes = 6000

var manifestNames = []string{
	"OCTOSUCKER.md",
	"AGENTS.md",
	"CLAUDE.md",
}

func Load(workspaceRoot string) string {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return ""
	}
	roots := candidateRoots(workspaceRoot)
	var sections []string
	seen := map[string]struct{}{}
	for _, root := range roots {
		for _, name := range manifestNames {
			path := filepath.Join(root, name)
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			text := strings.TrimSpace(string(data))
			if text == "" {
				continue
			}
			sections = append(sections, fmt.Sprintf("### %s\n%s", path, text))
		}
		if policy := verificationPolicy(root); policy != "" {
			sections = append(sections, fmt.Sprintf("### Auto-detected verification policy for %s\n%s", root, policy))
		}
	}
	return truncateRunes(strings.Join(sections, "\n\n"), maxContextRunes)
}

func candidateRoots(workspaceRoot string) []string {
	abs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		abs = workspaceRoot
	}
	abs = filepath.Clean(abs)
	roots := []string{abs}
	parent := filepath.Dir(abs)
	if parent != "" && parent != "." && parent != abs {
		roots = append(roots, parent)
	}
	return roots
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if max <= 0 || len(r) <= max {
		return s
	}
	return string(r[:max]) + "\n...[project context truncated]"
}

func verificationPolicy(root string) string {
	var lines []string
	if fileExists(filepath.Join(root, "go.mod")) {
		lines = append(lines, "- Go project: run `go test ./...` after runtime or library changes.")
	}
	if fileExists(filepath.Join(root, "package.json")) {
		lines = append(lines, "- Node project: inspect package.json scripts and run the narrowest relevant build/test script.")
	}
	if fileExists(filepath.Join(root, "Cargo.toml")) {
		lines = append(lines, "- Rust project: run `cargo test` after code changes.")
	}
	return strings.Join(lines, "\n")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
