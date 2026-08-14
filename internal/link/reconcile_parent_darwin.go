//go:build darwin

package link

import "golang.org/x/sys/unix"

func renameReconcileNoReplace(fd int, from, to string) error {
	return unix.RenameatxNp(fd, from, fd, to, unix.RENAME_EXCL)
}
