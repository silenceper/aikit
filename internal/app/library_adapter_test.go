package app

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/silenceper/aikit/internal/library"
)

type classificationRunner struct{ calls int }

func (runner *classificationRunner) Run(context.Context, string, ...string) (string, error) {
	runner.calls++
	return "", errors.New("runner reached")
}

func TestPrepareAddUsesSameSourceClassificationAsPreview(t *testing.T) {
	t.Run("existing local directory", func(t *testing.T) {
		runner := &classificationRunner{}
		source := t.TempDir()
		if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: demo\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		adapter := libraryAdapter{service: library.Service{LibraryRoot: t.TempDir(), CacheRoot: t.TempDir(), Runner: runner}}
		mutation, err := adapter.PrepareAdd(context.Background(), AddPrepareRequest{Source: source}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer mutation.Abort()
		if runner.calls != 0 || len(mutation.Entries()) != 1 {
			t.Fatalf("local prepare used runner=%d entries=%+v", runner.calls, mutation.Entries())
		}
	})

	t.Run("missing explicit relative local path", func(t *testing.T) {
		runner := &classificationRunner{}
		working := t.TempDir()
		old, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(working); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chdir(old) })
		adapter := libraryAdapter{service: library.Service{LibraryRoot: t.TempDir(), CacheRoot: t.TempDir(), Runner: runner}}
		_, err = adapter.PrepareAdd(context.Background(), AddPrepareRequest{Source: "./missing"}, nil)
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("missing local path error = %v, want filesystem not-exist", err)
		}
		if runner.calls != 0 {
			t.Fatalf("missing explicit local path invoked Git %d time(s)", runner.calls)
		}
	})

	for _, source := range []string{"https://example.test/acme/repo.git", "git@example.test:acme/repo.git"} {
		t.Run(source, func(t *testing.T) {
			runner := &classificationRunner{}
			adapter := libraryAdapter{service: library.Service{LibraryRoot: t.TempDir(), CacheRoot: t.TempDir(), Runner: runner}}
			_, err := adapter.PrepareAdd(context.Background(), AddPrepareRequest{Source: source}, nil)
			if err == nil || runner.calls == 0 {
				t.Fatalf("remote prepare error=%v runner calls=%d", err, runner.calls)
			}
		})
	}
}
