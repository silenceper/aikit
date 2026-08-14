package link

import (
	"fmt"
	"path/filepath"

	"github.com/silenceper/aikit/pkg/config"
)

type reconcileParent interface {
	Close() error
	State(string, string) (State, error)
	Fingerprint(string) (config.Fingerprint, error)
	MoveNoReplace(string, string) error
	Symlink(string, string) error
	Remove(string) error
	StillCurrent() (bool, error)
}

func reconcileBaseName(path string) (string, error) {
	name := filepath.Base(path)
	if name == "." || name == ".." || name == "" || filepath.Clean(name) != name {
		return "", fmt.Errorf("unsafe reconcile entry name")
	}
	return name, nil
}

func sameFingerprint(actual config.Fingerprint, expected *config.Fingerprint) bool {
	return expected != nil && actual.Kind == expected.Kind && actual.Hash == expected.Hash && actual.LinkTarget == expected.LinkTarget
}
