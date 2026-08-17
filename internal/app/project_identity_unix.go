//go:build darwin || linux

package app

import (
	"crypto/sha256"
	"fmt"
	"os"
	"syscall"
)

func projectPathIdentity(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("directory identity is unavailable")
	}
	payload := fmt.Sprintf("aikit-project-v1\x00%s\x00%d:%d", path, uint64(stat.Dev), uint64(stat.Ino))
	return fmt.Sprintf("v1:%x", sha256.Sum256([]byte(payload))), nil
}

func projectPathComponentUnsafe(os.FileInfo) bool { return false }
