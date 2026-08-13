package library

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestHashSkillDeterministicAndRecordsMetadata(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	writeTestFile(t, filepath.Join(a, "SKILL.md"), "hello", 0o644)
	writeTestFile(t, filepath.Join(a, "bin", "run"), "same", 0o755)
	writeTestFile(t, filepath.Join(b, "bin", "run"), "same", 0o755)
	writeTestFile(t, filepath.Join(b, "SKILL.md"), "hello", 0o644)

	ha, err := HashSkill(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := HashSkill(b)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Fatalf("same tree hashed differently: %s != %s", ha, hb)
	}

	if err := os.Chmod(filepath.Join(b, "bin", "run"), 0o644); err != nil {
		t.Fatal(err)
	}
	hMode, err := HashSkill(b)
	if err != nil {
		t.Fatal(err)
	}
	if hMode == ha {
		t.Fatal("executable bit was not included in hash")
	}

	writeTestFile(t, filepath.Join(b, "bin", "run"), "same!", 0o755)
	hContent, err := HashSkill(b)
	if err != nil {
		t.Fatal(err)
	}
	if hContent == ha {
		t.Fatal("content length/content was not included in hash")
	}
}

func TestHashSkillIgnoresGitDirectory(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "SKILL.md"), "body", 0o644)
	before, err := HashSkill(root)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, ".git", "objects", "ignored"), "noise", 0o755)
	after, err := HashSkill(root)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal(".git content changed skill hash")
	}
}

func TestHashSkillIncludesEmptyDirectories(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "SKILL.md"), "body", 0o644)
	before, err := HashSkill(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	after, err := HashSkill(root)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("empty directory was not included in hash")
	}
}

func TestHashAndCopyRejectEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	writeTestFile(t, filepath.Join(root, "SKILL.md"), "---\nname: safe\n---\n", 0o644)
	writeTestFile(t, outside, "secret", 0o644)
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := HashSkill(root); err == nil {
		t.Fatal("HashSkill accepted escaping symlink")
	}
	if err := AtomicCopy(root, filepath.Join(t.TempDir(), "dest")); err == nil {
		t.Fatal("AtomicCopy accepted escaping symlink")
	}
}

func TestHashAndCopyAllowInRootSymlink(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "SKILL.md"), "---\nname: safe\n---\n", 0o644)
	writeTestFile(t, filepath.Join(root, "docs", "guide"), "guide", 0o644)
	if err := os.Symlink("docs/guide", filepath.Join(root, "guide-link")); err != nil {
		t.Fatal(err)
	}
	if _, err := HashSkill(root); err != nil {
		t.Fatalf("HashSkill rejected contained symlink: %v", err)
	}
	dest := filepath.Join(t.TempDir(), "dest")
	if err := AtomicCopy(root, dest); err != nil {
		t.Fatalf("AtomicCopy rejected contained symlink: %v", err)
	}
	info, err := os.Lstat(filepath.Join(dest, "guide-link"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("copy did not preserve symlink")
	}
}

func TestSecureReadRejectsFileReplacedAfterInspection(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "SKILL.md")
	writeTestFile(t, path, "safe", 0o644)
	inspected, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	writeTestFile(t, outside, "must-not-read", 0o644)
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	content, err := secureReadRegular(root, "SKILL.md", inspected)
	if err == nil {
		t.Fatalf("secure read followed replacement and returned %q", content)
	}
}
