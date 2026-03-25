package execmcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func normalizeRoots(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(paths))
	seen := make(map[string]bool)
	for _, s := range paths {
		s = expandTilde(s)
		if s == "" {
			continue
		}
		abs, err := filepath.Abs(s)
		if err != nil {
			return nil, fmt.Errorf("invalid workspace_dir %q: %w", s, err)
		}
		abs = filepath.Clean(abs)
		st, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("workspace_dir %q: %w (directory must already exist)", abs, err)
		}
		if !st.IsDir() {
			return nil, fmt.Errorf("workspace_dir %q is not a directory", abs)
		}
		canonical, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return nil, fmt.Errorf("resolve workspace_dir %q: %w", abs, err)
		}
		canonical = filepath.Clean(canonical)
		if seen[canonical] {
			continue
		}
		seen[canonical] = true
		out = append(out, canonical)
	}
	return out, nil
}

func expandTilde(s string) string {
	if s == "~" || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return s
		}
		return home + s[1:]
	}
	return s
}

func resolveWorkDir(workDir string, roots []string) (string, error) {
	if len(roots) == 0 {
		return "", fmt.Errorf("execmcp: workspace_dirs not configured")
	}
	path := filepath.Clean(workDir)
	if path == "" || path == "/" {
		return "", fmt.Errorf("execmcp: working directory must be inside workspace_dirs, not empty or /")
	}
	var abs string
	if filepath.IsAbs(path) {
		var err error
		abs, err = filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("invalid work_dir %q: %w", workDir, err)
		}
		abs = filepath.Clean(abs)
	} else {
		abs = filepath.Clean(filepath.Join(roots[0], path))
	}
	for _, root := range roots {
		rootAbs := filepath.Clean(root)
		rel, err := filepath.Rel(rootAbs, abs)
		if err != nil {
			continue
		}
		if rel != "." && (strings.HasPrefix(rel, "..") || filepath.IsAbs(rel)) {
			continue
		}
		canonical, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("work_dir %q does not exist or cannot be resolved: %w", workDir, err)
		}
		canonical = filepath.Clean(canonical)
		for _, r := range roots {
			if isUnderRoot(canonical, filepath.Clean(r)) {
				// Always return canonical path to prevent symlink cwd escape.
				return canonical, nil
			}
		}
		return "", fmt.Errorf("work_dir %q resolves outside workspace_dirs", workDir)
	}
	return "", fmt.Errorf("work_dir %q is outside allowed workspace_dirs", workDir)
}

func isUnderRoot(canonicalPath, canonicalRoot string) bool {
	rel, err := filepath.Rel(canonicalRoot, canonicalPath)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func resolvePathInWorkspace(path string, workDir string, roots []string) (string, error) {
	if len(roots) == 0 {
		return "", fmt.Errorf("execmcp: workspace_dirs not configured")
	}
	path = filepath.Clean(path)
	if path == "" || path == "/" || path == ".." || strings.HasPrefix(path, "../") {
		return "", fmt.Errorf("path %q is not allowed", path)
	}
	var abs string
	if filepath.IsAbs(path) {
		abs = filepath.Clean(path)
	} else {
		abs = filepath.Clean(filepath.Join(workDir, path))
	}
	for _, root := range roots {
		rootAbs := filepath.Clean(root)
		rel, err := filepath.Rel(rootAbs, abs)
		if err != nil {
			continue
		}
		if rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)) {
			canonical, err := filepath.EvalSymlinks(abs)
			if err != nil {
				return "", fmt.Errorf("path %q does not exist or cannot be resolved: %w", path, err)
			}
			for _, r := range roots {
				if isUnderRoot(canonical, filepath.Clean(r)) {
					return canonical, nil
				}
			}
			return "", fmt.Errorf("path %q resolves outside workspace_dirs", path)
		}
	}
	return "", fmt.Errorf("path %q is outside allowed workspace_dirs", path)
}
