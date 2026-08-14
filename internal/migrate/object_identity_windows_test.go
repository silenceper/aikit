//go:build windows

package migrate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsObjectIdentityUsesStableFileID(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	for _, path := range []string{first, second} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	firstInfo, err := os.Lstat(first)
	if err != nil {
		t.Fatal(err)
	}
	secondInfo, err := os.Lstat(second)
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := objectIdentity(first, firstInfo)
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := objectIdentity(second, secondInfo)
	if err != nil {
		t.Fatal(err)
	}
	if firstID == "" || secondID == "" || firstID == secondID {
		t.Fatalf("Windows identities = %q, %q", firstID, secondID)
	}
}
