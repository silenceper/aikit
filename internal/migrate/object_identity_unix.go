//go:build !windows

package migrate

import (
	"fmt"
	"os"
	"syscall"
)

func objectIdentity(_ string, info os.FileInfo) (string, error) {
	if info == nil {
		return "", fmt.Errorf("file identity is unavailable")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("file identity is unavailable on this platform")
	}
	return fmt.Sprintf("%d:%d", uint64(stat.Dev), uint64(stat.Ino)), nil
}
