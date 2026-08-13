package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeSourceEquivalentGitHubForms(t *testing.T) {
	forms := []string{
		"vercel-labs/agent-skills",
		"https://github.com/vercel-labs/agent-skills.git",
		"git@github.com:vercel-labs/agent-skills.git",
		"ssh://git@github.com/vercel-labs/agent-skills.git",
	}
	for _, form := range forms {
		got, err := NormalizeSource(form)
		if err != nil {
			t.Fatalf("NormalizeSource(%q): %v", form, err)
		}
		if got != "vercel-labs/agent-skills" {
			t.Errorf("NormalizeSource(%q) = %q", form, got)
		}
	}
}

func TestNormalizeSourcePreservesHostPortSubgroupsAndEncoding(t *testing.T) {
	got, err := NormalizeSource("https://GitLab.Example.COM:8443/group/sub group/répo.git")
	if err != nil {
		t.Fatal(err)
	}
	if want := "gitlab.example.com:8443/group/sub%20group/r%C3%A9po"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeSourceCanonicalizesExistingPercentEncoding(t *testing.T) {
	got, err := NormalizeSource("git@gitlab.com:group/sub/repo%2fpart.git")
	if err != nil {
		t.Fatal(err)
	}
	if want := "gitlab.com/group/sub/repo%2Fpart"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSafeLibraryPathRejectsUnsafeIDs(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"", ".", "..", "/absolute", "a/../b", "a//b", "a\\b", "a/\x00b"} {
		if _, err := SafeLibraryPath(root, id); err == nil {
			t.Errorf("SafeLibraryPath accepted %q", id)
		}
	}
	got, err := SafeLibraryPath(root, "gitlab.com/group/repo/skill")
	if err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(resolvedRoot, "gitlab.com", "group", "repo", "skill")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestValidateSourcePath(t *testing.T) {
	for _, path := range []string{"/abs", "../escape", "a/../../escape", "a\\..\\escape", ""} {
		if err := ValidateSourcePath(path); err == nil {
			t.Errorf("accepted unsafe source path %q", path)
		}
	}
	for _, path := range []string{".", "skills/code-review", "group/sub/skill"} {
		if err := ValidateSourcePath(path); err != nil {
			t.Errorf("rejected source path %q: %v", path, err)
		}
	}
}

func TestSafeLibraryPathRejectsEscapingIntermediateSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := SafeLibraryPath(root, "escape/skill"); err == nil {
		t.Fatal("accepted an intermediate symlink outside the library")
	}
}

func TestRepoCachePathDeterministicAndContained(t *testing.T) {
	root := t.TempDir()
	one, err := RepoCachePath(root, "gitlab.example.com/group/repo")
	if err != nil {
		t.Fatal(err)
	}
	two, err := RepoCachePath(root, "gitlab.example.com/group/repo")
	if err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if one != two || !isWithin(resolvedRoot, one) || filepath.Dir(one) != filepath.Join(resolvedRoot, "repos") {
		t.Fatalf("unsafe or unstable cache path %q %q", one, two)
	}
}

func TestRepoCachePathRejectsEscapingReposSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "repos")); err != nil {
		t.Fatal(err)
	}
	if _, err := RepoCachePath(root, "acme/repo"); err == nil {
		t.Fatal("accepted repos symlink outside cache root")
	}
}
