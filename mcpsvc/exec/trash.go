package execmcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const trashDirName = ".trash"

var rmFlags = map[string]bool{
	"-f": true, "-r": true, "-rf": true, "-fr": true,
	"-i": true, "-v": true, "--interactive": true, "--verbose": true,
	"-d": true, "--dir": true, "-P": true,
}

func isRmCommand(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	base := filepath.Base(argv[0])
	return base == "rm"
}

func parseRmPaths(argv []string) []string {
	var paths []string
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			paths = append(paths, argv[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && rmFlags[arg] {
			continue
		}
		paths = append(paths, arg)
	}
	return paths
}

func moveToTrash(workDir string, absPaths []string) (moved []string, err error) {
	trashRoot := filepath.Join(workDir, trashDirName)
	if err := os.MkdirAll(trashRoot, 0755); err != nil {
		return nil, fmt.Errorf("create trash dir: %w", err)
	}
	base := time.Now().UnixNano()
	for i, p := range absPaths {
		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return moved, fmt.Errorf("stat %q: %w", p, err)
		}
		name := filepath.Base(p)
		if name == "" || name == "." || name == ".." {
			continue
		}
		if info.IsDir() {
			name = name + "_dir"
		}
		unique := fmt.Sprintf("%d-%s", base+int64(i), name)
		dest := filepath.Join(trashRoot, unique)
		if err := os.Rename(p, dest); err != nil {
			return moved, fmt.Errorf("move %q to trash: %w", p, err)
		}
		moved = append(moved, p)
	}
	return moved, nil
}
