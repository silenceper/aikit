//go:build darwin

package library

import "golang.org/x/sys/unix"

func moveNoReplace(from, to string) error {
	return unix.RenamexNp(from, to, unix.RENAME_EXCL)
}
