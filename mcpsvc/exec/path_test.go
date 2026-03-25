package execmcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWorkDirRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, err := resolveWorkDir("escape", []string{root})
	if err == nil {
		t.Fatalf("expected error for symlink escape, got nil")
	}
}

func TestResolveWorkDirReturnsCanonicalPathInsideRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "safe")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatalf("mkdir safe dir: %v", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("eval root: %v", err)
	}

	link := filepath.Join(root, "safe-link")
	if err := os.Symlink(inside, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	got, err := resolveWorkDir("safe-link", []string{canonicalRoot})
	if err != nil {
		t.Fatalf("resolve work dir: %v", err)
	}
	want, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("eval symlink: %v", err)
	}
	if got != want {
		t.Fatalf("expected canonical %q, got %q", want, got)
	}
}
