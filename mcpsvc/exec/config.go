package execmcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Roots []string
	// HostBindRoots: host paths for nested docker -v (same order as Roots), derived from EXEC_HOST_REPO_DIR
	// and paths under MCP_EXEC_REPO_MOUNT (Docker Compose deployment only).
	HostBindRoots []string
	TimeoutSec    int
	Blacklist     []string
	Container     ContainerConfig
}

type ContainerConfig struct {
	Runtime      string
	Image        string
	Name         string
	WorkspaceDir string
	ReadOnlyRoot bool
	User         string
}

func LoadFromEnv() (Config, error) {
	raw := os.Getenv("EXEC_WORKSPACE_DIRS")
	if raw == "" {
		return Config{}, fmt.Errorf("EXEC_WORKSPACE_DIRS is required (comma-separated absolute or ~ paths)")
	}
	parts := strings.Split(raw, ",")
	var paths []string
	for _, p := range parts {
		if p != "" {
			paths = append(paths, p)
		}
	}
	roots, err := normalizeRoots(paths)
	if err != nil {
		return Config{}, err
	}
	if len(roots) == 0 {
		return Config{}, fmt.Errorf("EXEC_WORKSPACE_DIRS: no valid paths after normalization")
	}
	composeMount, err := composeRepoMountFromEnv()
	if err != nil {
		return Config{}, err
	}
	if !workspaceRootsUnderComposeMount(roots, composeMount) {
		return Config{}, fmt.Errorf("EXEC_WORKSPACE_DIRS must be under MCP_EXEC_REPO_MOUNT=%s (Docker Compose layout)", composeMount)
	}
	hr := strings.TrimSpace(os.Getenv("EXEC_HOST_REPO_DIR"))
	if hr == "" {
		return Config{}, fmt.Errorf("EXEC_HOST_REPO_DIR is required (host absolute path to repo root, same as compose volume left side)")
	}
	hrCanon := filepath.Clean(expandTilde(hr))
	if hrCanon == composeMount {
		return Config{}, fmt.Errorf("EXEC_HOST_REPO_DIR is the host path (compose volume left side); it must not equal MCP_EXEC_REPO_MOUNT (%q)", composeMount)
	}
	hostBind, err := hostBindRootsFromHostRepo(roots, hr, composeMount)
	if err != nil {
		return Config{}, err
	}
	if err := validateComposeNestedBinds(roots, hostBind, composeMount); err != nil {
		return Config{}, err
	}
	timeout := 30
	if s := os.Getenv("EXEC_COMMAND_TIMEOUT_SEC"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("EXEC_COMMAND_TIMEOUT_SEC: invalid integer")
		}
		timeout = n
	}
	var blacklist []string
	if s := os.Getenv("EXEC_COMMAND_BLACKLIST"); s != "" {
		for _, frag := range strings.Split(s, ",") {
			if frag != "" {
				blacklist = append(blacklist, frag)
			}
		}
	}
	runtime := os.Getenv("EXEC_CONTAINER_RUNTIME")
	if runtime == "" {
		runtime = "docker"
	}
	image := os.Getenv("EXEC_CONTAINER_IMAGE")
	if image == "" {
		image = "octosucker-exec-sandbox:latest"
	}
	name := os.Getenv("EXEC_CONTAINER_NAME")
	if name == "" {
		name = "octosucker-agent-sandbox"
	}
	workspaceDir := os.Getenv("EXEC_CONTAINER_WORKSPACE_DIR")
	if workspaceDir == "" {
		workspaceDir = "/workspace"
	}
	readOnlyRoot := true
	if v := os.Getenv("EXEC_CONTAINER_READONLY_ROOT"); v != "" {
		readOnlyRoot = parseBool(v)
	}
	containerUser := os.Getenv("EXEC_CONTAINER_USER")
	if containerUser == "" {
		containerUser = "65532:65532"
	}
	return Config{
		Roots:         roots,
		HostBindRoots: hostBind,
		TimeoutSec:    timeout,
		Blacklist:     blacklist,
		Container: ContainerConfig{
			Runtime:      runtime,
			Image:        image,
			Name:         name,
			WorkspaceDir: workspaceDir,
			ReadOnlyRoot: readOnlyRoot,
			User:         containerUser,
		},
	}, nil
}

func parseBool(v string) bool {
	return v == "1" || strings.EqualFold(v, "true")
}

// composeRepoMountFromEnv is the in-container path where the repo is bind-mounted (docker-compose volume RHS).
func composeRepoMountFromEnv() (string, error) {
	m := strings.TrimSpace(os.Getenv("MCP_EXEC_REPO_MOUNT"))
	if m == "" {
		return "", fmt.Errorf("MCP_EXEC_REPO_MOUNT is required (absolute path inside mcp-exec container; compose volume right side)")
	}
	c := filepath.Clean(m)
	if !filepath.IsAbs(c) {
		return "", fmt.Errorf("MCP_EXEC_REPO_MOUNT must be absolute (got %q)", m)
	}
	return c, nil
}

// validateComposeNestedBinds catches the common mistake of setting EXEC_HOST_REPO_DIR to the
// in-container mount path (e.g. /repo): hostBind would equal roots and docker would try to bind a non-host path.
func validateComposeNestedBinds(roots, hostBind []string, composeMount string) error {
	if len(roots) != len(hostBind) {
		return fmt.Errorf("internal: roots/hostBind length mismatch")
	}
	for i := range roots {
		r := filepath.Clean(roots[i])
		h := filepath.Clean(hostBind[i])
		if r == h {
			return fmt.Errorf("nested docker -v would use container path %q as bind source; set EXEC_HOST_REPO_DIR to the host repo root (compose volume left side), not MCP_EXEC_REPO_MOUNT (%q)", r, composeMount)
		}
	}
	return nil
}

func workspaceRootsUnderComposeMount(roots []string, composeMount string) bool {
	composeMount = filepath.Clean(composeMount)
	sep := string(filepath.Separator)
	for _, r := range roots {
		r = filepath.Clean(r)
		if r == composeMount || strings.HasPrefix(r, composeMount+sep) {
			return true
		}
	}
	return false
}

// hostBindRootsFromHostRepo maps each root under composeMount to hostRepo/<relative suffix> for nested docker -v.
func hostBindRootsFromHostRepo(roots []string, hostRepo, composeMount string) ([]string, error) {
	composeMount = filepath.Clean(composeMount)
	hostRepo = filepath.Clean(expandTilde(strings.TrimSpace(hostRepo)))
	if hostRepo == "" || hostRepo == "." {
		return nil, fmt.Errorf("EXEC_HOST_REPO_DIR must be an absolute host path (got empty or .)")
	}
	if !filepath.IsAbs(hostRepo) {
		return nil, fmt.Errorf("EXEC_HOST_REPO_DIR must be absolute (got %q)", hostRepo)
	}
	sep := string(filepath.Separator)
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		r = filepath.Clean(r)
		switch {
		case r == composeMount:
			out = append(out, hostRepo)
		case strings.HasPrefix(r, composeMount+sep):
			suffix := strings.TrimPrefix(r, composeMount+sep)
			out = append(out, filepath.Join(hostRepo, suffix))
		default:
			return nil, fmt.Errorf("with EXEC_HOST_REPO_DIR=%q, workspace roots must stay under %s; got %q", hostRepo, composeMount, r)
		}
	}
	return out, nil
}
