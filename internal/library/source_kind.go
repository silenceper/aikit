package library

import (
	"fmt"
	"os"
	"strings"
)

type AddSourceKind string

const (
	AddSourceLocal  AddSourceKind = "local"
	AddSourceRemote AddSourceKind = "remote"
)

// ClassifyAddSource recognizes unambiguous Git transports before consulting
// the filesystem. Ambiguous owner/repo shorthand retains the existing add
// behavior: an existing path is local, otherwise it is GitHub shorthand.
func ClassifyAddSource(source string) (AddSourceKind, error) {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" || strings.ContainsRune(trimmed, '\x00') {
		return "", fmt.Errorf("source is empty or contains NUL")
	}
	if strings.Contains(trimmed, "://") || looksLikeSCPSource(trimmed) {
		if _, err := NormalizeSource(trimmed); err != nil {
			return "", err
		}
		return AddSourceRemote, nil
	}
	if info, err := os.Stat(trimmed); err == nil {
		if !info.IsDir() {
			return AddSourceLocal, fmt.Errorf("local source %q is not a directory", source)
		}
		return AddSourceLocal, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if _, err := NormalizeSource(trimmed); err == nil {
		return AddSourceRemote, nil
	}
	return AddSourceLocal, nil
}
