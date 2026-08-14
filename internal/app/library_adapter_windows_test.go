//go:build windows

package app

import (
	"context"
	"testing"

	"github.com/silenceper/aikit/internal/library"
)

func TestPrepareAddWindowsRemoteFormsReachGitWithoutPathStat(t *testing.T) {
	for _, source := range []string{
		"https://example.test/acme/repo.git",
		"ssh://git@example.test/acme/repo.git",
		"git@example.test:acme/repo.git",
	} {
		runner := &classificationRunner{}
		adapter := libraryAdapter{service: library.Service{LibraryRoot: t.TempDir(), CacheRoot: t.TempDir(), Runner: runner}}
		_, err := adapter.PrepareAdd(context.Background(), AddPrepareRequest{Source: source}, nil)
		if err == nil || runner.calls == 0 {
			t.Fatalf("remote %q error=%v runner calls=%d", source, err, runner.calls)
		}
	}
}
