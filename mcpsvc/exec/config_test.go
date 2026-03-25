package execmcp

import (
	"path/filepath"
	"testing"
)

const testComposeMount = "/octoplus"

func TestHostBindRootsFromHostRepo(t *testing.T) {
	roots := []string{filepath.Clean("/octoplus/agent/workspace")}
	got, err := hostBindRootsFromHostRepo(roots, "/Users/me/OctoSucker", testComposeMount)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/Users/me/OctoSucker", "agent", "workspace")
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %v want [%s]", got, want)
	}
}

func TestHostBindRootsFromHostRepoExactMount(t *testing.T) {
	roots := []string{testComposeMount}
	got, err := hostBindRootsFromHostRepo(roots, "/Users/me/proj", testComposeMount)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "/Users/me/proj" {
		t.Fatalf("got %v", got)
	}
}

func TestHostBindRootsFromHostRepoRejectsRelative(t *testing.T) {
	_, err := hostBindRootsFromHostRepo([]string{"/octoplus/a"}, "relative", testComposeMount)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateComposeNestedBindsRejectsEqualPaths(t *testing.T) {
	roots := []string{filepath.Clean("/repo/agent/workspace")}
	hostBind := []string{filepath.Clean("/repo/agent/workspace")}
	err := validateComposeNestedBinds(roots, hostBind, "/repo")
	if err == nil {
		t.Fatal("expected error when host bind equals container path")
	}
}

func TestValidateComposeNestedBindsOK(t *testing.T) {
	roots := []string{filepath.Clean("/octoplus/agent/workspace")}
	hostBind := []string{filepath.Join("/Users/me", "OctoSucker", "agent", "workspace")}
	if err := validateComposeNestedBinds(roots, hostBind, "/octoplus"); err != nil {
		t.Fatal(err)
	}
}

func TestHostBindRootsFromHostRepoCustomMount(t *testing.T) {
	const mount = "/repo"
	roots := []string{filepath.Clean("/repo/agent/workspace")}
	got, err := hostBindRootsFromHostRepo(roots, "/Users/me/OctoSucker", mount)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/Users/me/OctoSucker", "agent", "workspace")
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %v want [%s]", got, want)
	}
}
