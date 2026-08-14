//go:build darwin || linux

package link

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silenceper/aikit/pkg/config"
	"golang.org/x/sys/unix"
)

type unixReconcileParent struct {
	fd           int
	path         string
	resolvedPath string
	device       uint64
	inode        uint64
}

func openReconcileParent(op config.PendingOperation) (reconcileParent, error) {
	return openReconcileParentMode(op, true)
}

func openReconcileParentReadOnly(op config.PendingOperation) (reconcileParent, error) {
	return openReconcileParentMode(op, false)
}

func openReconcileParentMode(op config.PendingOperation, create bool) (reconcileParent, error) {
	base := op.Scope.ProjectPath
	if op.Scope.Project == "" {
		var err error
		base, err = os.UserHomeDir()
		if err != nil {
			return nil, err
		}
	}
	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return nil, fmt.Errorf("resolve reconcile scope base: %w", err)
	}
	parentPath := filepath.Dir(op.Target)
	relative, err := filepath.Rel(base, parentPath)
	if err != nil || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("reconcile parent escapes scope base")
	}
	fd, err := unix.Open(resolvedBase, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = unix.Close(fd)
		}
	}()
	if relative != "." {
		for _, component := range strings.Split(relative, string(filepath.Separator)) {
			if component == "" || component == "." || component == ".." {
				return nil, fmt.Errorf("unsafe reconcile parent component")
			}
			next, openErr := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if create && errors.Is(openErr, unix.ENOENT) {
				if mkdirErr := unix.Mkdirat(fd, component, 0o755); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
					return nil, mkdirErr
				}
				next, openErr = unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			}
			if openErr != nil {
				return nil, openErr
			}
			_ = unix.Close(fd)
			fd = next
		}
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, err
	}
	closeOnError = false
	return &unixReconcileParent{fd: fd, path: parentPath, resolvedPath: filepath.Join(resolvedBase, relative), device: uint64(stat.Dev), inode: stat.Ino}, nil
}

func (p *unixReconcileParent) Close() error { return unix.Close(p.fd) }

func (p *unixReconcileParent) readlink(name string) (string, error) {
	buffer := make([]byte, 4096)
	n, err := unix.Readlinkat(p.fd, name, buffer)
	if err != nil {
		return "", err
	}
	if n == len(buffer) {
		return "", fmt.Errorf("reconcile symlink target is too long")
	}
	return string(buffer[:n]), nil
}

func (p *unixReconcileParent) State(name, libraryRoot string) (State, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(p.fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return State{Kind: StateAbsent}, nil
		}
		return State{}, err
	}
	switch stat.Mode & unix.S_IFMT {
	case unix.S_IFLNK:
		raw, err := p.readlink(name)
		if err != nil {
			return State{}, err
		}
		resolved := raw
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(p.resolvedPath, resolved)
		}
		resolved, err = filepath.EvalSymlinks(resolved)
		if err != nil {
			if os.IsNotExist(err) {
				return State{Kind: StateExternalLink, LinkTarget: raw, Broken: true}, nil
			}
			return State{}, err
		}
		root, err := filepath.EvalSymlinks(libraryRoot)
		if err != nil {
			return State{}, err
		}
		id, managed := containedID(root, resolved)
		if !managed {
			return State{Kind: StateExternalLink, LinkTarget: raw}, nil
		}
		return State{Kind: StateManagedLink, SkillID: id, LinkTarget: raw}, nil
	case unix.S_IFDIR:
		return State{Kind: StateDirectory}, nil
	default:
		return State{Kind: StateFile}, nil
	}
}

func (p *unixReconcileParent) Fingerprint(name string) (config.Fingerprint, error) {
	raw, err := p.readlink(name)
	if err != nil {
		return config.Fingerprint{}, err
	}
	resolved := raw
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(p.resolvedPath, resolved)
	}
	content, err := FingerprintPath(resolved)
	if err != nil {
		return config.Fingerprint{}, err
	}
	return config.Fingerprint{Kind: "symlink", Hash: content.Hash, LinkTarget: raw}, nil
}

func (p *unixReconcileParent) MoveNoReplace(from, to string) error {
	return renameReconcileNoReplace(p.fd, from, to)
}

func (p *unixReconcileParent) Symlink(target, name string) error {
	return unix.Symlinkat(target, p.fd, name)
}

func (p *unixReconcileParent) Remove(name string) error {
	return unix.Unlinkat(p.fd, name, 0)
}

func (p *unixReconcileParent) StillCurrent() (bool, error) {
	var current unix.Stat_t
	if err := unix.Stat(p.path, &current); err != nil {
		return false, err
	}
	return uint64(current.Dev) == p.device && current.Ino == p.inode, nil
}
