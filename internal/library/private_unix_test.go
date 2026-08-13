//go:build !windows

package library

import (
	"os"
	"testing"
)

func TestHardenPrivateDirectoryTightensUnixPermissions(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := hardenPrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("private directory mode = %o, want 700", info.Mode().Perm())
	}
}
