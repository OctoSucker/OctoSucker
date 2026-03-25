package execmcp

import (
	"os"
	"path/filepath"
	"strings"
)

// hostMntBindSource maps macOS-style absolute paths to paths visible to dockerd inside
// Docker Desktop's Linux VM (/host_mnt + path). Caller must only invoke this when the
// operator has explicitly enabled nested bind translation (see bindSourceForNestedDocker).
func hostMntBindSource(src string) string {
	src = filepath.ToSlash(filepath.Clean(src))
	if strings.HasPrefix(src, "/host_mnt/") {
		return src
	}
	for _, prefix := range []string{"/Users/", "/Volumes/", "/private/var/", "/private/tmp/"} {
		if strings.HasPrefix(src, prefix) {
			return "/host_mnt" + src
		}
	}
	return src
}

func envTruthy(k string) bool {
	s := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	return s == "1" || s == "true" || s == "yes"
}

// bindSourceForNestedDocker returns the left-hand path for nested `docker run -v`.
// When EXEC_NESTED_DOCKER_USE_HOST_MNT_PREFIX is set (1/true/yes), applies hostMntBindSource
// for Docker Desktop; otherwise returns src unchanged. No implicit /.dockerenv detection.
func bindSourceForNestedDocker(src string) string {
	src = filepath.ToSlash(filepath.Clean(src))
	if !envTruthy("EXEC_NESTED_DOCKER_USE_HOST_MNT_PREFIX") {
		return src
	}
	return hostMntBindSource(src)
}
