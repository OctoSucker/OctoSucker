package execmcp

import (
	"path/filepath"
	"testing"
)

func TestMapWorkDirToContainer(t *testing.T) {
	rootA := filepath.Clean("/tmp/work-a")
	rootB := filepath.Clean("/tmp/work-b")

	got, err := mapWorkDirToContainer(filepath.Join(rootB, "sub"), []string{rootA, rootB}, "/workspace")
	if err != nil {
		t.Fatalf("mapWorkDirToContainer returned error: %v", err)
	}
	want := "/workspace/1/sub"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestMapWorkDirToContainerRejectsOutsideRoots(t *testing.T) {
	_, err := mapWorkDirToContainer("/etc", []string{"/tmp/work-a"}, "/workspace")
	if err == nil {
		t.Fatalf("expected error for out-of-root work dir")
	}
}
