package library

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolveAddSourceUnderstandsSkillsSHAndGitHubShorthand(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantSource string
		wantSkills []string
	}{
		{
			name:       "exact skills.sh page",
			source:     "https://skills.sh/vercel-labs/agent-skills/find-skills",
			wantSource: "https://github.com/vercel-labs/agent-skills.git",
			wantSkills: []string{"find-skills"},
		},
		{
			name:       "skills.sh repository page",
			source:     "https://skills.sh/vercel-labs/agent-skills",
			wantSource: "https://github.com/vercel-labs/agent-skills.git",
		},
		{
			name:       "github shorthand",
			source:     "vercel-labs/agent-skills",
			wantSource: "https://github.com/vercel-labs/agent-skills.git",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveAddSource(test.source)
			if err != nil {
				t.Fatal(err)
			}
			if got.Kind != AddSourceRemote || got.Source != test.wantSource || !reflect.DeepEqual(got.SuggestedSelections, test.wantSkills) {
				t.Fatalf("ResolveAddSource(%q) = %#v", test.source, got)
			}
		})
	}
}

func TestResolveAddSourceRejectsUnsafeOrAmbiguousSkillsSHRoutes(t *testing.T) {
	for _, source := range []string{
		"http://skills.sh/owner/repo/skill",
		"https://user:secret@skills.sh/owner/repo/skill",
		"https://user:secret@example.test/owner/repo.git",
		"https://skills.sh/owner/repo/skill?x=1",
		"https://skills.sh/owner/repo/skill#fragment",
		"https://skills.sh/owner/repo/skill/extra",
		"https://skills.sh/owner/repo/%2e%2e",
		"https://skills.sh/owner/repo/a%2fb",
		"https://skills.sh/owner/.git/skill",
	} {
		t.Run(source, func(t *testing.T) {
			if _, err := ResolveAddSource(source); err == nil {
				t.Fatalf("ResolveAddSource(%q) succeeded", source)
			}
		})
	}
}

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
