//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package config

import "fmt"

func replaceFile(_, _ string) error {
	return fmt.Errorf("atomic config replacement is unsupported on this platform")
}
