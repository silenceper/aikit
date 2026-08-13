//go:build !windows

package library

import (
	"fmt"
	"os"
)

func hardenPrivateDirectory(path string) error {
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("make cache directory private: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		return fmt.Errorf("cache directory %q is not private", path)
	}
	return nil
}
