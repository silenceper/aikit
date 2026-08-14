//go:build darwin || linux

package library

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type unixVerifiedSkillRoot struct{ directory *os.File }

func openVerifiedSkillRoot(libraryRoot, id string, beforeComponent func(int) error) (VerifiedSkillRoot, error) {
	if _, err := lexicalLibraryPath(libraryRoot, id); err != nil {
		return nil, err
	}
	absRoot, err := filepath.Abs(libraryRoot)
	if err != nil {
		return nil, err
	}
	current, err := unix.Open(filepath.Clean(absRoot), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open library root without following links: %w", err)
	}
	for index, segment := range strings.Split(id, "/") {
		if beforeComponent != nil {
			if err := beforeComponent(index); err != nil {
				_ = unix.Close(current)
				return nil, err
			}
		}
		next, err := unix.Openat(current, segment, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		_ = unix.Close(current)
		if err != nil {
			return nil, fmt.Errorf("open skill id component without following links: %w", err)
		}
		current = next
	}
	return &unixVerifiedSkillRoot{directory: os.NewFile(uintptr(current), id)}, nil
}

func (root *unixVerifiedSkillRoot) Close() error { return root.directory.Close() }

func (root *unixVerifiedSkillRoot) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	segments := []string{"."}
	if name != "." {
		segments = strings.Split(name, "/")
	}
	current, err := unix.Openat(int(root.directory.Fd()), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	if name == "." {
		return os.NewFile(uintptr(current), name), nil
	}
	for index, segment := range segments {
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
		if index < len(segments)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, openErr := unix.Openat(current, segment, flags, 0)
		_ = unix.Close(current)
		if openErr != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: openErr}
		}
		current = next
	}
	return os.NewFile(uintptr(current), name), nil
}

func (root *unixVerifiedSkillRoot) Readlink(name string) (string, error) {
	if !fs.ValidPath(name) || name == "." {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrInvalid}
	}
	current, err := unix.Openat(int(root.directory.Fd()), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return "", err
	}
	defer func() { _ = unix.Close(current) }()
	segments := strings.Split(name, "/")
	for _, segment := range segments[:len(segments)-1] {
		next, openErr := unix.Openat(current, segment, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil {
			return "", &fs.PathError{Op: "readlink", Path: name, Err: openErr}
		}
		_ = unix.Close(current)
		current = next
	}
	buffer := make([]byte, 256)
	for {
		n, readErr := unix.Readlinkat(current, segments[len(segments)-1], buffer)
		if readErr != nil {
			return "", &fs.PathError{Op: "readlink", Path: name, Err: readErr}
		}
		if n < len(buffer) {
			return string(buffer[:n]), nil
		}
		buffer = make([]byte, len(buffer)*2)
	}
}
