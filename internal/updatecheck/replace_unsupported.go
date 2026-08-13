//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package updatecheck

import "fmt"

func replaceFile(string, string) error {
	return fmt.Errorf("atomic cache replacement is unsupported on this platform")
}
