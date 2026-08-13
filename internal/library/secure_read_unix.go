//go:build darwin || linux

package library

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func secureReadRegular(root, relative string, expected os.FileInfo) ([]byte, error) {
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(rootFD)
	currentFD := rootFD
	ownedFD := -1
	defer func() {
		if ownedFD >= 0 {
			_ = unix.Close(ownedFD)
		}
	}()
	segments := strings.Split(filepath.ToSlash(relative), "/")
	for _, segment := range segments[:len(segments)-1] {
		if segment == "" || segment == "." || segment == ".." {
			return nil, fmt.Errorf("unsafe source path %q", relative)
		}
		next, err := unix.Openat(currentFD, segment, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return nil, fmt.Errorf("open source directory without following links: %w", err)
		}
		if ownedFD >= 0 {
			_ = unix.Close(ownedFD)
		}
		ownedFD = next
		currentFD = next
	}
	name := segments[len(segments)-1]
	fd, err := unix.Openat(currentFD, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open source file without following links: %w", err)
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open source file")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() {
		return nil, fmt.Errorf("source file changed type")
	}
	if err := sameSourceFile(expected, opened); err != nil {
		return nil, err
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err := sameSourceFile(opened, after); err != nil {
		return nil, fmt.Errorf("source changed while reading: %w", err)
	}
	return content, nil
}

func sameSourceFile(expected, actual os.FileInfo) error {
	want, err := fileIdentity(expected)
	if err != nil {
		return err
	}
	got, err := fileIdentity(actual)
	if err != nil {
		return err
	}
	if want != got || expected.Size() != actual.Size() || !expected.ModTime().Equal(actual.ModTime()) {
		return fmt.Errorf("source file identity changed")
	}
	return nil
}
