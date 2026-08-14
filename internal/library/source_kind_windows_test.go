//go:build windows

package library

import "testing"

func TestClassifyAddSourceWindowsRemoteFormsDoNotReachPathStat(t *testing.T) {
	for _, source := range []string{
		"https://example.test/acme/repo.git",
		"ssh://git@example.test/acme/repo.git",
		"git@example.test:acme/repo.git",
	} {
		kind, err := ClassifyAddSource(source)
		if err != nil || kind != AddSourceRemote {
			t.Fatalf("ClassifyAddSource(%q) = %q, %v", source, kind, err)
		}
	}
}
