//go:build !darwin && !linux && !windows

package app

import (
	"fmt"
	"os"
)

func projectPathIdentity(string) (string, error) {
	return "", fmt.Errorf("strong project directory identity is unsupported on this platform")
}

func projectPathComponentUnsafe(os.FileInfo) bool { return true }
