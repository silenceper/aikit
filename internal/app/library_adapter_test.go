package app

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silenceper/aikit/internal/library"
)

type previewRunner struct {
	calls [][]string
}

func (runner *previewRunner) Run(_ context.Context, dir string, args ...string) (string, error) {
	runner.calls = append(runner.calls, append([]string{dir}, args...))
	switch args[0] {
	case "clone":
		return "", os.MkdirAll(args[len(args)-1], 0o755)
	case "symbolic-ref":
		return "main\n", nil
	case "checkout":
		for _, item := range []string{"find-skills", "other"} {
			path := filepath.Join(dir, "skills", item)
			if err := os.MkdirAll(path, 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("---\nname: "+item+"\n---\n"), 0o644); err != nil {
				return "", err
			}
		}
		return "", nil
	case "rev-parse":
		return strings.Repeat("a", 40) + "\n", nil
	default:
		return "", nil
	}
}

func TestPreviewAddSkillsSHDefersNetworkThenReturnsCandidates(t *testing.T) {
	runner := &previewRunner{}
	libraryRoot, cacheRoot := filepath.Join(t.TempDir(), "library"), filepath.Join(t.TempDir(), "cache")
	adapter := libraryAdapter{service: library.Service{LibraryRoot: libraryRoot, CacheRoot: cacheRoot, Runner: runner}}
	source := "https://skills.sh/vercel-labs/agent-skills/find-skills"

	offline, err := adapter.Preview(context.Background(), AddPreviewRequest{Source: source})
	if err != nil {
		t.Fatal(err)
	}
	if !offline.NetworkRequired || offline.ResolvedSource != "https://github.com/vercel-labs/agent-skills.git" || len(offline.SuggestedSelections) != 1 || offline.SuggestedSelections[0] != "find-skills" || len(runner.calls) != 0 {
		t.Fatalf("offline preview = %+v, runner calls=%d", offline, len(runner.calls))
	}

	online, err := adapter.Preview(context.Background(), AddPreviewRequest{Source: source, AllowNetwork: true})
	if err != nil {
		t.Fatal(err)
	}
	if online.NetworkRequired || len(online.Candidates) != 2 || online.Ref == nil || online.Resolved != strings.Repeat("a", 40) {
		t.Fatalf("online preview = %+v", online)
	}
	if online.ResolvedSource != offline.ResolvedSource || len(online.SuggestedSelections) != 1 || online.SuggestedSelections[0] != "find-skills" {
		t.Fatalf("online source/suggestion changed: %+v", online)
	}
	for _, path := range []string{libraryRoot, cacheRoot} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("network preview wrote persistent path %q: %v", path, err)
		}
	}
	stale, err := adapter.Preview(context.Background(), AddPreviewRequest{
		Source: "https://skills.sh/vercel-labs/agent-skills/missing", AllowNetwork: true,
	})
	if err != nil {
		t.Fatalf("stale skills.sh suggestion should degrade to candidate selection: %v", err)
	}
	if len(stale.Candidates) != 2 || len(stale.SuggestedSelections) != 0 || len(stale.Warnings) != 1 || !strings.Contains(stale.Warnings[0], "missing") {
		t.Fatalf("stale skills.sh preview = %+v", stale)
	}
}

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
