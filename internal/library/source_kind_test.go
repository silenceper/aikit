package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyAddSourceRecognizesRemoteBeforeFilesystemLookup(t *testing.T) {
	for _, source := range []string{
		"https://example.test/acme/repo.git",
		"ssh://git@example.test/acme/repo.git",
		"git@example.test:acme/repo.git",
	} {
		t.Run(source, func(t *testing.T) {
			kind, err := ClassifyAddSource(source)
			if err != nil || kind != AddSourceRemote {
				t.Fatalf("ClassifyAddSource(%q) = %q, %v", source, kind, err)
			}
		})
	}
}

func TestClassifyAddSourcePreservesExistingLocalAndGitShorthandSemantics(t *testing.T) {
	working := t.TempDir()
	local := filepath.Join(working, "owner", "repo")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(working); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	if kind, err := ClassifyAddSource("owner/repo"); err != nil || kind != AddSourceLocal {
		t.Fatalf("existing owner/repo = %q, %v", kind, err)
	}
	if kind, err := ClassifyAddSource("missing/repo"); err != nil || kind != AddSourceRemote {
		t.Fatalf("missing owner/repo shorthand = %q, %v", kind, err)
	}
	if kind, err := ClassifyAddSource("./missing"); err != nil || kind != AddSourceLocal {
		t.Fatalf("explicit relative local = %q, %v", kind, err)
	}
}
