//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package updatecheck

import "os"

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}
