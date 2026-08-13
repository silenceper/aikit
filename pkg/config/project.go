package config

import (
	"fmt"
	"os"
	"path/filepath"
)

func canonicalProjectPath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("project path must be absolute: %q", path)
	}
	abs := filepath.Clean(path)
	candidate := abs
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve project path %q: %w", path, err)
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
		missing = append(missing, filepath.Base(candidate))
		candidate = parent
	}
	return abs, nil
}
