package library

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenVerifiedSkillRootRejectsIntermediateSwap(t *testing.T) {
	libraryRoot := t.TempDir()
	local := filepath.Join(libraryRoot, "local")
	inside := filepath.Join(local, "demo")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inside, "SKILL.md"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	secret := "outside sentinel"
	if err := os.WriteFile(filepath.Join(outside, "demo", "SKILL.md"), []byte(secret), 0o644); err != nil {
		t.Fatal(err)
	}

	root, err := openVerifiedSkillRoot(libraryRoot, "local/demo", func(component int) error {
		if component != 0 {
			return nil
		}
		if err := os.Rename(local, local+".original"); err != nil {
			return err
		}
		return os.Symlink(outside, local)
	})
	if root != nil {
		defer root.Close()
		file, openErr := root.Open("SKILL.md")
		if openErr == nil {
			content, _ := io.ReadAll(file)
			_ = file.Close()
			if strings.Contains(string(content), secret) {
				t.Fatalf("outside SKILL.md was read: %q", content)
			}
		}
	}
	if err == nil {
		t.Fatal("intermediate symlink swap was accepted")
	}
}
