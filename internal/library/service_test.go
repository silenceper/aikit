package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/silenceper/aikit/pkg/config"
)

func TestDiscoverReadsFrontmatterAndNestedSkills(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "skills", "one", "SKILL.md"), "---\nname: first\ndescription: First skill\n---\nbody", 0o644)
	writeTestFile(t, filepath.Join(root, "group", "two", "SKILL.md"), "body", 0o644)
	candidates, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("got %d candidates", len(candidates))
	}
	if candidates[0].RelativePath != "group/two" || candidates[0].Name != "two" {
		t.Fatalf("unexpected first candidate: %#v", candidates[0])
	}
	if candidates[1].RelativePath != "skills/one" || candidates[1].Name != "first" || candidates[1].Description != "First skill" {
		t.Fatalf("unexpected second candidate: %#v", candidates[1])
	}
}

func TestDiscoverGitIncludesRootAndNestedSkills(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "SKILL.md"), "---\nname: root\n---\n", 0o644)
	writeTestFile(t, filepath.Join(root, "nested", "SKILL.md"), "---\nname: nested\n---\n", 0o644)
	local, err := Discover(root)
	if err != nil || len(local) != 1 {
		t.Fatalf("local discover = %#v, %v", local, err)
	}
	gitCandidates, err := DiscoverGit(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(gitCandidates) != 2 {
		t.Fatalf("git discover got %d candidates", len(gitCandidates))
	}
}

func TestPreviewGitDiscoversInTemporaryCheckoutWithoutPersistentWrites(t *testing.T) {
	libraryRoot := filepath.Join(t.TempDir(), "library")
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	runner := &fakeRunner{resolved: strings.Repeat("a", 40)}
	runnerCustomWrite = func(dir string) {
		writeTestFileForRunner(filepath.Join(dir, "skills", "one", "SKILL.md"), "---\nname: one\n---\none")
		writeTestFileForRunner(filepath.Join(dir, "skills", "two", "SKILL.md"), "---\nname: two\n---\ntwo")
	}
	defer func() { runnerCustomWrite = nil }()

	preview, err := (Service{LibraryRoot: libraryRoot, CacheRoot: cacheRoot, Runner: runner}).PreviewGit(
		context.Background(), "https://github.com/acme/skills.git", "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Candidates) != 2 || preview.Candidates[0].Name != "one" || preview.Candidates[1].Name != "two" {
		t.Fatalf("unexpected candidates: %#v", preview.Candidates)
	}
	if preview.Ref == nil || preview.Ref.Kind != "branch" || preview.Ref.Value != "main" || preview.Resolved != strings.Repeat("a", 40) {
		t.Fatalf("unexpected ref/resolved: %#v", preview)
	}
	for _, candidate := range preview.Candidates {
		if candidate.Root != "" {
			t.Fatalf("temporary checkout leaked through candidate root: %#v", candidate)
		}
	}
	for _, path := range []string{libraryRoot, cacheRoot} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("preview created persistent path %q: %v", path, err)
		}
	}
	cloneDestination := runner.invocations[0][len(runner.invocations[0])-1]
	if _, err := os.Stat(cloneDestination); !os.IsNotExist(err) {
		t.Fatalf("temporary checkout was not removed: %q, %v", cloneDestination, err)
	}
}

type cancelledPreviewRunner struct{ destination string }

func (runner *cancelledPreviewRunner) Run(ctx context.Context, _ string, args ...string) (string, error) {
	if len(args) > 0 && args[0] == "clone" {
		runner.destination = args[len(args)-1]
		if err := os.MkdirAll(runner.destination, 0o755); err != nil {
			return "", err
		}
	}
	return "", ctx.Err()
}

func TestPreviewGitCancellationRemovesTemporaryCheckout(t *testing.T) {
	runner := &cancelledPreviewRunner{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Service{Runner: runner}).PreviewGit(ctx, "https://github.com/acme/skills.git", "", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PreviewGit cancellation = %v", err)
	}
	if runner.destination == "" {
		t.Fatal("runner was not reached")
	}
	if _, err := os.Stat(runner.destination); !os.IsNotExist(err) {
		t.Fatalf("cancelled preview left temporary checkout %q: %v", runner.destination, err)
	}
}

func TestPrepareGitSkillsSHSelectsPageSkillAndExplicitSelectionOverrides(t *testing.T) {
	runnerCustomWrite = func(dir string) {
		writeTestFileForRunner(filepath.Join(dir, "skills", "find-skills", "SKILL.md"), "---\nname: find-skills\n---\nfind")
		writeTestFileForRunner(filepath.Join(dir, "skills", "other", "SKILL.md"), "---\nname: other\n---\nother")
	}
	defer func() { runnerCustomWrite = nil }()
	for _, test := range []struct {
		name       string
		selections []string
		want       string
	}{
		{name: "page suggestion", want: "find-skills"},
		{name: "explicit override", selections: []string{"other"}, want: "other"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeRunner{resolved: strings.Repeat("a", 40)}
			service := Service{LibraryRoot: t.TempDir(), CacheRoot: t.TempDir(), Runner: runner}
			mutation, err := service.PrepareGit(context.Background(), "https://skills.sh/vercel-labs/agent-skills/find-skills", GitAddOptions{Skills: test.selections})
			if err != nil {
				t.Fatal(err)
			}
			defer mutation.Abort()
			if len(mutation.Skills) != 1 || mutation.Skills[0].Name != test.want {
				t.Fatalf("prepared skills = %+v", mutation.Skills)
			}
			if !runner.sawArgument("https://github.com/vercel-labs/agent-skills.git") {
				t.Fatalf("clone did not use resolved GitHub source: %+v", runner.invocations)
			}
		})
	}
}

func TestPrepareGitUsesFilteredMirrorAndManagedWorktree(t *testing.T) {
	runnerCustomWrite = func(dir string) {
		writeTestFileForRunner(filepath.Join(dir, "skills", "frontend-design", "SKILL.md"), "---\nname: frontend-design\n---\nbody")
	}
	defer func() { runnerCustomWrite = nil }()
	runner := &fakeRunner{resolved: strings.Repeat("a", 40)}
	service := Service{LibraryRoot: t.TempDir(), CacheRoot: t.TempDir(), Runner: runner}

	mutation, err := service.PrepareGit(context.Background(), "https://github.com/anthropics/skills.git", GitAddOptions{Skills: []string{"frontend-design"}})
	if err != nil {
		t.Fatal(err)
	}
	defer mutation.Abort()

	var filteredMirror, worktree bool
	for _, invocation := range runner.invocations {
		if slices.Contains(invocation, "--mirror") && slices.Contains(invocation, "--filter=blob:none") {
			filteredMirror = true
		}
		if slices.Contains(invocation, "worktree") && slices.Contains(invocation, "add") {
			worktree = true
		}
	}
	if !filteredMirror {
		t.Fatalf("persistent clone did not use a filtered mirror: %+v", runner.invocations)
	}
	if !worktree {
		t.Fatalf("filtered mirror was not checked out through a managed worktree: %+v", runner.invocations)
	}
}

func TestPrepareGitSkipsExactDuplicateAndRejectsChangedDuplicate(t *testing.T) {
	const skillBody = "---\nname: frontend-design\n---\nbody"
	fixture := t.TempDir()
	writeTestFile(t, filepath.Join(fixture, "skills", "frontend-design", "SKILL.md"), skillBody, 0o644)
	writeTestFile(t, filepath.Join(fixture, "skills", "new-skill", "SKILL.md"), "---\nname: new-skill\n---\nnew", 0o644)
	discovered, err := DiscoverGit(fixture)
	if err != nil || len(discovered) != 2 {
		t.Fatalf("discover fixture = %+v, %v", discovered, err)
	}
	var existingHash string
	for _, candidate := range discovered {
		if candidate.Name == "frontend-design" {
			existingHash = candidate.Hash
		}
	}
	existing := config.Skill{
		ID: "anthropics/skills/frontend-design", Name: "frontend-design",
		Source: "anthropics/skills", SourcePath: "skills/frontend-design",
		Ref: &config.Ref{Kind: "branch", Value: "main"}, Resolved: strings.Repeat("a", 40), Hash: existingHash,
	}
	libraryRoot := t.TempDir()
	writeTestFile(t, filepath.Join(libraryRoot, filepath.FromSlash(existing.ID), "SKILL.md"), skillBody, 0o644)
	runnerCustomWrite = func(dir string) {
		writeTestFileForRunner(filepath.Join(dir, "skills", "frontend-design", "SKILL.md"), skillBody)
		writeTestFileForRunner(filepath.Join(dir, "skills", "new-skill", "SKILL.md"), "---\nname: new-skill\n---\nnew")
	}
	defer func() { runnerCustomWrite = nil }()
	service := Service{LibraryRoot: libraryRoot, CacheRoot: t.TempDir(), Runner: &fakeRunner{resolved: strings.Repeat("b", 40)}}

	mutation, err := service.PrepareGit(context.Background(), "https://github.com/anthropics/skills.git", GitAddOptions{
		Skills: []string{"frontend-design", "new-skill"}, Existing: []config.Skill{existing},
	})
	if err != nil {
		t.Fatalf("exact duplicate should be a no-op: %v", err)
	}
	defer mutation.Abort()
	if len(mutation.Skills) != 2 || mutation.Skills[0].ID != existing.ID {
		t.Fatalf("duplicate entries = %+v", mutation.Skills)
	}
	if err := mutation.Commit(context.Background()); err != nil {
		t.Fatalf("duplicate no-op commit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(libraryRoot, "anthropics", "skills", "new-skill", "SKILL.md")); err != nil {
		t.Fatalf("new skill was not added beside skipped duplicate: %v", err)
	}

	changed := existing
	changed.Hash = "different-hash"
	_, err = service.PrepareGit(context.Background(), "https://github.com/anthropics/skills.git", GitAddOptions{
		Skills: []string{"frontend-design"}, Existing: []config.Skill{changed},
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "use update") {
		t.Fatalf("changed duplicate error = %v, want update guidance", err)
	}
}

func TestDiscoverRejectsInvalidExplicitName(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "SKILL.md"), "---\nname: ../escape\n---\n", 0o644)
	if _, err := Discover(root); err == nil {
		t.Fatal("Discover accepted invalid frontmatter name")
	}
}

func TestDiscoverRejectsExplicitEmptyOrNullName(t *testing.T) {
	for _, value := range []string{`""`, "null", "~"} {
		t.Run(value, func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, filepath.Join(root, "SKILL.md"), "---\nname: "+value+"\n---\n", 0o644)
			if _, err := Discover(root); err == nil {
				t.Fatal("accepted explicit empty frontmatter name")
			}
		})
	}
}

func TestDiscoverRejectsEscapingSkillFileBeforeReading(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "SKILL.md")
	writeTestFile(t, outside, "---\nname: leaked\n---\n", 0o000)
	if err := os.Symlink(outside, filepath.Join(root, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(root); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("expected containment error before parsing, got %v", err)
	}
}

func TestDiscoverSelectionRejectsResolvedSourcePathEscape(t *testing.T) {
	checkout := t.TempDir()
	outside := t.TempDir()
	writeTestFile(t, filepath.Join(outside, "SKILL.md"), "---\nname: outside\n---\n", 0o644)
	if err := os.Symlink(outside, filepath.Join(checkout, "linked-skill")); err != nil {
		t.Fatal(err)
	}
	if _, err := discoverSelection(checkout, "linked-skill", "", true); err == nil {
		t.Fatal("accepted source_path resolving outside checkout")
	}
}

func TestAddLocalReusesNameAndHashOtherwiseAllocatesStableID(t *testing.T) {
	libraryRoot := t.TempDir()
	svc := Service{LibraryRoot: libraryRoot, CacheRoot: t.TempDir()}
	one := filepath.Join(t.TempDir(), "one")
	writeTestFile(t, filepath.Join(one, "SKILL.md"), "---\nname: demo\n---\nsame", 0o644)
	added, err := svc.AddLocal(one, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0].ID != "local/demo" {
		t.Fatalf("unexpected add: %#v", added)
	}

	duplicate, err := svc.AddLocal(one, added)
	if err != nil {
		t.Fatal(err)
	}
	if len(duplicate) != 1 || duplicate[0].ID != added[0].ID {
		t.Fatalf("did not reuse existing id: %#v", duplicate)
	}

	two := filepath.Join(t.TempDir(), "two")
	writeTestFile(t, filepath.Join(two, "SKILL.md"), "---\nname: demo\n---\ndifferent", 0o644)
	second, err := svc.AddLocal(two, added)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(second[0].ID, "local/demo-") || len(strings.TrimPrefix(second[0].ID, "local/demo-")) != 12 {
		t.Fatalf("unexpected collision id %q", second[0].ID)
	}
}

func TestAllocateLocalIsStableForSortedOrigins(t *testing.T) {
	candidates := []LocalCandidate{
		{Origin: "p/z/cursor", Candidate: Candidate{Name: "demo", Hash: strings.Repeat("b", 64)}},
		{Origin: "g/cursor", Candidate: Candidate{Name: "demo", Hash: strings.Repeat("a", 64)}},
	}
	allocated, err := AllocateLocal(candidates, nil)
	if err != nil {
		t.Fatal(err)
	}
	if allocated[0].Origin != "g/cursor" || allocated[0].Skill.ID != "local/demo" {
		t.Fatalf("unstable first allocation: %#v", allocated)
	}
	if allocated[1].Skill.ID != "local/demo-"+strings.Repeat("b", 12) {
		t.Fatalf("unexpected second allocation: %#v", allocated)
	}
}

func TestAllocateLocalUsesSpecifiedAgentOrder(t *testing.T) {
	candidates := []LocalCandidate{
		{Origin: "g/claude-code", Candidate: Candidate{Name: "demo", Hash: strings.Repeat("b", 64)}},
		{Origin: "g/cursor", Candidate: Candidate{Name: "demo", Hash: strings.Repeat("a", 64)}},
	}
	allocated, err := AllocateLocal(candidates, nil)
	if err != nil {
		t.Fatal(err)
	}
	if allocated[0].Origin != "g/cursor" || allocated[0].Skill.ID != "local/demo" {
		t.Fatalf("did not use Agent table order: %#v", allocated)
	}
}

func TestAddLocalRejectsOrphanDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "---\nname: demo\n---\nnew", 0o644)
	writeTestFile(t, filepath.Join(root, "local", "demo", "SKILL.md"), "orphan", 0o644)
	if _, err := (Service{LibraryRoot: root}).AddLocal(source, nil); err == nil {
		t.Fatal("local add accepted orphan destination")
	}
}

func TestAddLocalRejectsDanglingOrphanBeforeCopy(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "---\nname: demo\n---\nnew", 0o644)
	destination := filepath.Join(root, "local", "demo")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), destination); err != nil {
		t.Fatal(err)
	}
	copyCalled := false
	svc := Service{LibraryRoot: root, CopySkill: func(_, _ string) error {
		copyCalled = true
		return nil
	}}
	if _, err := svc.AddLocal(source, nil); err == nil || !strings.Contains(err.Error(), "without a matching ledger") {
		t.Fatalf("dangling orphan was not rejected clearly: %v", err)
	}
	if copyCalled {
		t.Fatal("copy started before dangling orphan rejection")
	}
}

type fakeRunner struct {
	root        string
	resolved    string
	fail        string
	name        string
	empty       bool
	invocations [][]string
}

var runnerCustomWrite func(dir string)

func (f *fakeRunner) Run(_ context.Context, dir string, args ...string) (string, error) {
	f.invocations = append(f.invocations, append([]string{dir}, args...))
	if len(args) == 0 {
		return "", errors.New("missing command")
	}
	if args[0] == f.fail {
		return "", errors.New("injected failure")
	}
	switch args[0] {
	case "clone":
		dest := args[len(args)-1]
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return "", err
		}
		return "", nil
	case "fetch":
		return "", nil
	case "worktree":
		if len(args) < 2 {
			return "", errors.New("missing worktree command")
		}
		switch args[1] {
		case "prune":
			return "", nil
		case "add":
			dest := args[len(args)-2]
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return "", err
			}
			f.populateCheckout(dest)
			return "", nil
		case "remove":
			return "", os.RemoveAll(args[len(args)-1])
		default:
			return "", fmt.Errorf("unsupported worktree command %q", args[1])
		}
	case "symbolic-ref":
		return "origin/main\n", nil
	case "checkout":
		f.populateCheckout(dir)
		return "", nil
	case "rev-parse":
		return f.resolved + "\n", nil
	default:
		return "", nil
	}
}

func (f *fakeRunner) populateCheckout(dir string) {
	if runnerCustomWrite != nil {
		runnerCustomWrite(dir)
		return
	}
	if f.empty {
		return
	}
	name := f.name
	if name == "" {
		name = "remote-skill"
	}
	writeTestFileForRunner(filepath.Join(dir, "packages", "skill", "SKILL.md"), "---\nname: "+name+"\n---\nremote")
}

func (f *fakeRunner) sawArgument(value string) bool {
	for _, invocation := range f.invocations {
		for _, argument := range invocation {
			if argument == value {
				return true
			}
		}
	}
	return false
}

func writeTestFileForRunner(path, content string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(content), 0o644)
}

func TestAddGitStoresCanonicalSourceRefFullObjectAndSourcePath(t *testing.T) {
	runner := &fakeRunner{resolved: strings.Repeat("a", 40)}
	svc := Service{LibraryRoot: t.TempDir(), CacheRoot: t.TempDir(), Runner: runner}
	items, err := svc.AddGit(context.Background(), "https://github.com/acme/repo.git", GitAddOptions{SourcePath: "packages/skill"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items", len(items))
	}
	got := items[0]
	if got.ID != "acme/repo/remote-skill" || got.Source != "acme/repo" || got.SourcePath != "packages/skill" {
		t.Fatalf("unexpected source identity: %#v", got)
	}
	if got.Ref == nil || got.Ref.Kind != "branch" || got.Ref.Value != "main" || got.Resolved != strings.Repeat("a", 40) {
		t.Fatalf("unexpected ref: %#v", got)
	}
}

func TestAddGitSanitizesAndLimitsNormalizeSourceErrors(t *testing.T) {
	source := "https://alice:supersecret@example.com/%zz" + strings.Repeat("界", 2000)
	_, err := (Service{}).AddGit(context.Background(), source, GitAddOptions{})
	if err == nil {
		t.Fatal("malformed source was accepted")
	}
	diagnostic := err.Error()
	for _, secret := range []string{"alice", "supersecret"} {
		if strings.Contains(diagnostic, secret) {
			t.Fatalf("normalize source error leaked %q: %q", secret, diagnostic)
		}
	}
	if len([]byte(diagnostic)) > maxGitDiagnosticBytes || !utf8.ValidString(diagnostic) {
		t.Fatalf("normalize source error is not bounded valid UTF-8: bytes=%d", len([]byte(diagnostic)))
	}
}

func TestAddGitRejectsHTTPSourceCredentialsBeforePersistingOrigin(t *testing.T) {
	runner := &fakeRunner{resolved: strings.Repeat("a", 40)}
	svc := Service{LibraryRoot: t.TempDir(), CacheRoot: t.TempDir(), Runner: runner}
	for _, source := range []string{
		"https://alice:secret@example.com/acme/repo.git",
		"https://example.com/acme/repo.git?access_token=secret",
	} {
		if _, err := svc.AddGit(context.Background(), source, GitAddOptions{}); err == nil {
			t.Fatalf("credential-bearing source %q was accepted", source)
		}
	}
	if len(runner.invocations) != 0 {
		t.Fatalf("credential-bearing source reached git runner: %#v", runner.invocations)
	}
}

func TestAddGitRejectsOrphanDestination(t *testing.T) {
	runner := &fakeRunner{resolved: strings.Repeat("a", 40)}
	svc := Service{LibraryRoot: t.TempDir(), CacheRoot: t.TempDir(), Runner: runner}
	dest, err := SafeLibraryPath(svc.LibraryRoot, "acme/repo/remote-skill")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dest, "SKILL.md"), "orphan", 0o644)
	if _, err := svc.AddGit(context.Background(), "acme/repo", GitAddOptions{SourcePath: "packages/skill"}); err == nil {
		t.Fatal("overwrote orphan destination without force")
	}
}

func TestAddGitForceStillRejectsOrphanDestination(t *testing.T) {
	runner := &fakeRunner{resolved: strings.Repeat("a", 40)}
	svc := Service{LibraryRoot: t.TempDir(), CacheRoot: t.TempDir(), Runner: runner}
	dest, err := SafeLibraryPath(svc.LibraryRoot, "acme/repo/remote-skill")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dest, "SKILL.md"), "orphan", 0o644)
	if _, err := svc.AddGit(context.Background(), "acme/repo", GitAddOptions{SourcePath: "packages/skill", Force: true}); err == nil || !strings.Contains(err.Error(), "without a matching ledger") {
		t.Fatalf("force accepted orphan destination: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil || string(content) != "orphan" {
		t.Fatalf("orphan changed: %q, %v", content, err)
	}
}

func TestAddGitRejectsRepositoryWithoutSkills(t *testing.T) {
	runner := &fakeRunner{resolved: strings.Repeat("a", 40), empty: true}
	svc := Service{LibraryRoot: t.TempDir(), CacheRoot: t.TempDir(), Runner: runner}
	if _, err := svc.AddGit(context.Background(), "acme/repo", GitAddOptions{}); err == nil || !strings.Contains(err.Error(), "no SKILL.md") {
		t.Fatalf("expected no skills error, got %v", err)
	}
}

func TestAddGitBatchRollbackRestoresEveryOldDestination(t *testing.T) {
	root := t.TempDir()
	runner := &fakeRunner{resolved: strings.Repeat("b", 40)}
	svc := Service{LibraryRoot: root, CacheRoot: t.TempDir(), Runner: runner}
	for _, name := range []string{"one", "two"} {
		dest, err := SafeLibraryPath(root, "acme/repo/"+name)
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(dest, "SKILL.md"), "old-"+name, 0o644)
	}
	runnerCustomWrite = func(dir string) {
		writeTestFileForRunner(filepath.Join(dir, "one", "SKILL.md"), "---\nname: one\n---\nnew")
		writeTestFileForRunner(filepath.Join(dir, "two", "SKILL.md"), "---\nname: two\n---\nnew")
	}
	defer func() { runnerCustomWrite = nil }()
	calls := 0
	svc.Rename = func(old, new string) error {
		calls++
		if calls == 4 {
			return errors.New("injected rename failure")
		}
		return os.Rename(old, new)
	}
	existing := []config.Skill{{ID: "acme/repo/one"}, {ID: "acme/repo/two"}}
	if _, err := svc.AddGit(context.Background(), "acme/repo", GitAddOptions{Existing: existing, Force: true}); err == nil {
		t.Fatal("expected batch failure")
	}
	for _, name := range []string{"one", "two"} {
		content, err := os.ReadFile(filepath.Join(root, "acme", "repo", name, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "old-"+name {
			t.Fatalf("%s was not rolled back: %q", name, content)
		}
	}
}

func TestBatchRollbackAtEveryCommitStage(t *testing.T) {
	for _, failCall := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprintf("rename-%d", failCall), func(t *testing.T) {
			root := t.TempDir()
			var specs []CopySpec
			for _, name := range []string{"one", "two"} {
				source := filepath.Join(t.TempDir(), name)
				writeTestFile(t, filepath.Join(source, "SKILL.md"), "new-"+name, 0o644)
				destination := filepath.Join(root, name)
				writeTestFile(t, filepath.Join(destination, "SKILL.md"), "old-"+name, 0o644)
				specs = append(specs, CopySpec{Source: source, Destination: destination})
			}
			calls := 0
			batch, err := PrepareBatch(specs, nil, func(old, new string) error {
				calls++
				if calls == failCall {
					return errors.New("injected rename failure")
				}
				return os.Rename(old, new)
			})
			if err != nil {
				t.Fatal(err)
			}
			defer batch.Abort()
			if err := batch.Commit(); err == nil {
				t.Fatal("expected commit failure")
			}
			for _, name := range []string{"one", "two"} {
				content, err := os.ReadFile(filepath.Join(root, name, "SKILL.md"))
				if err != nil || string(content) != "old-"+name {
					t.Fatalf("%s was not restored: %q, %v", name, content, err)
				}
			}
		})
	}
}

func TestBatchSyncFailureRollsBack(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	destination := filepath.Join(root, "skill")
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "new", 0o644)
	writeTestFile(t, filepath.Join(destination, "SKILL.md"), "old", 0o644)
	batch, err := PrepareBatch([]CopySpec{{Source: source, Destination: destination}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer batch.Abort()
	batch.SyncDirectory = func(string) error { return errors.New("injected sync failure") }
	if err := batch.Commit(); err == nil {
		t.Fatal("expected sync failure")
	}
	content, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil || string(content) != "old" {
		t.Fatalf("old destination was not restored: %q, %v", content, err)
	}
}

func TestBatchRollbackNeverDeletesConcurrentDestinationReplacement(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	destination := filepath.Join(root, "skill")
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "new", 0o644)
	writeTestFile(t, filepath.Join(destination, "SKILL.md"), "old", 0o644)
	batch, err := PrepareBatch([]CopySpec{{Source: source, Destination: destination}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer batch.Abort()
	laterAside := filepath.Join(root, "installed-aside")
	batch.SyncDirectory = func(string) error {
		if err := os.Rename(destination, laterAside); err != nil {
			return err
		}
		writeTestFile(t, filepath.Join(destination, "SKILL.md"), "later-user-content", 0o644)
		return errors.New("injected sync failure after replacement")
	}
	if err := batch.Commit(); err == nil {
		t.Fatal("expected commit failure")
	}
	content, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil || string(content) != "later-user-content" {
		t.Fatalf("rollback clobbered concurrent destination: %q, %v", content, err)
	}
	if batch.Copies[0].Backup == "" {
		t.Fatal("rollback lost backup location")
	}
	old, err := os.ReadFile(filepath.Join(batch.Copies[0].Backup, "SKILL.md"))
	if err != nil || string(old) != "old" {
		t.Fatalf("rollback did not preserve old backup: %q, %v", old, err)
	}
}

func TestBatchAbortNeverDeletesConcurrentStagingReplacement(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "new", 0o644)
	batch, err := PrepareBatch([]CopySpec{{Source: source, Destination: filepath.Join(root, "skill")}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	stage := batch.Copies[0].Staging
	if err := os.Rename(stage, stage+"-owned-aside"); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(stage, "SKILL.md"), "later-user-content", 0o644)
	batch.Abort()
	content, err := os.ReadFile(filepath.Join(stage, "SKILL.md"))
	if err != nil || string(content) != "later-user-content" {
		t.Fatalf("abort deleted concurrent staging replacement: %q, %v", content, err)
	}
}

func TestPrepareBatchCopyFailureCleansAllStaging(t *testing.T) {
	root := t.TempDir()
	sources := []string{filepath.Join(t.TempDir(), "one"), filepath.Join(t.TempDir(), "two")}
	for _, source := range sources {
		writeTestFile(t, filepath.Join(source, "SKILL.md"), "body", 0o644)
	}
	calls := 0
	_, err := PrepareBatch([]CopySpec{
		{Source: sources[0], Destination: filepath.Join(root, "one")},
		{Source: sources[1], Destination: filepath.Join(root, "two")},
	}, func(source, destination string) error {
		calls++
		if calls == 2 {
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return err
			}
			return errors.New("injected copy failure")
		}
		return AtomicCopy(source, destination)
	}, nil)
	if err == nil {
		t.Fatal("expected prepare failure")
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("staging leaked after prepare failure: %#v", entries)
	}
}

func TestUpdateRefFailureKeepsOldLibraryAndSkillValue(t *testing.T) {
	root := t.TempDir()
	old := config.Skill{
		ID: "acme/repo/remote-skill", Name: "remote-skill", Source: "acme/repo", SourcePath: "packages/skill",
		Ref: &config.Ref{Kind: "branch", Value: "main"}, Resolved: strings.Repeat("a", 40),
	}
	dest, err := SafeLibraryPath(root, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dest, "SKILL.md"), "old-content", 0o644)
	runner := &fakeRunner{resolved: strings.Repeat("b", 40), name: "renamed"}
	svc := Service{LibraryRoot: root, CacheRoot: t.TempDir(), Runner: runner}
	updated, err := svc.UpdateRef(context.Background(), old, nil)
	if err == nil {
		t.Fatal("UpdateRef accepted changed skill name")
	}
	if updated.Resolved != old.Resolved || updated.Ref.Value != old.Ref.Value {
		t.Fatalf("returned mutated old skill: %#v", updated)
	}
	content, readErr := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "old-content" {
		t.Fatalf("old library replaced on failure: %q", content)
	}
}

func TestUpdateRefCheckoutAndCopyFailuresKeepOldResult(t *testing.T) {
	for _, test := range []struct {
		name        string
		fail        string
		resolved    string
		copyFailure bool
	}{
		{name: "checkout", fail: "worktree", resolved: strings.Repeat("b", 40)},
		{name: "short object id", resolved: "abc123"},
		{name: "copy", resolved: strings.Repeat("b", 40), copyFailure: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			old := config.Skill{
				ID: "acme/repo/remote-skill", Name: "remote-skill", Source: "acme/repo", SourcePath: "packages/skill",
				Ref: &config.Ref{Kind: "branch", Value: "main"}, Resolved: strings.Repeat("a", 40),
			}
			dest, err := SafeLibraryPath(root, old.ID)
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, filepath.Join(dest, "SKILL.md"), "old-content", 0o644)
			svc := Service{LibraryRoot: root, CacheRoot: t.TempDir(), Runner: &fakeRunner{resolved: test.resolved, fail: test.fail}}
			if test.copyFailure {
				svc.CopySkill = func(_, _ string) error { return errors.New("injected copy failure") }
			}
			updated, updateErr := svc.UpdateRef(context.Background(), old, nil)
			if updateErr == nil {
				t.Fatal("expected update error")
			}
			if test.name == "short object id" && strings.Contains(updateErr.Error(), test.resolved) {
				t.Fatalf("invalid remote output leaked into diagnostic: %v", updateErr)
			}
			if updated.Resolved != old.Resolved || updated.Ref.Value != old.Ref.Value {
				t.Fatalf("returned mutated old skill: %#v", updated)
			}
			content, readErr := os.ReadFile(filepath.Join(dest, "SKILL.md"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(content) != "old-content" {
				t.Fatalf("old library replaced on failure: %q", content)
			}
		})
	}
}

func TestUpdateRefUsesOnlyStoredSourcePath(t *testing.T) {
	root := t.TempDir()
	old := config.Skill{
		ID: "acme/repo/remote-skill", Name: "remote-skill", Source: "acme/repo", SourcePath: "packages/skill",
		Ref: &config.Ref{Kind: "branch", Value: "main"}, Resolved: strings.Repeat("a", 40),
	}
	runner := &fakeRunner{resolved: strings.Repeat("b", 40)}
	svc := Service{LibraryRoot: root, CacheRoot: t.TempDir(), Runner: runner}
	updated, err := svc.UpdateRef(context.Background(), old, &config.Ref{Kind: "branch", Value: "next"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Ref.Value != "next" || updated.Resolved != strings.Repeat("b", 40) {
		t.Fatalf("unexpected updated skill: %#v", updated)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(old.ID), "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateRefReusesPersistentSSHOriginAndMissingAmbiguousCacheFails(t *testing.T) {
	runner := &fakeRunner{resolved: strings.Repeat("a", 40)}
	svc := Service{LibraryRoot: t.TempDir(), CacheRoot: t.TempDir(), Runner: runner}
	items, err := svc.AddGit(context.Background(), "ssh://git@gitlab.example.com:2222/group/repo.git", GitAddOptions{SourcePath: "packages/skill"})
	if err != nil {
		t.Fatal(err)
	}
	runner.invocations = nil
	runner.resolved = strings.Repeat("b", 40)
	if _, err := svc.UpdateRef(context.Background(), items[0], nil); err != nil {
		t.Fatal(err)
	}
	if runner.sawArgument("https://gitlab.example.com:2222/group/repo.git") {
		t.Fatal("update reconstructed an incorrect HTTPS URL")
	}

	missing := Service{LibraryRoot: t.TempDir(), CacheRoot: t.TempDir(), Runner: runner}
	if _, err := missing.UpdateRef(context.Background(), items[0], nil); err == nil || !strings.Contains(err.Error(), "re-add") {
		t.Fatalf("missing ambiguous cache did not fail clearly: %v", err)
	}
}

func TestGitArgumentsRedactURLCredentials(t *testing.T) {
	got := sanitizedArgs([]string{"clone", "https://user:secret@example.com/repo.git"})
	if strings.Contains(got, "secret") || strings.Contains(got, "user") {
		t.Fatalf("credentials leaked from sanitized args: %q", got)
	}
}

func TestGitOutputRedactsURLCredentials(t *testing.T) {
	got := sanitizeOutput("fatal: https://user:secret@example.com/repo.git failed")
	if strings.Contains(got, "secret") || strings.Contains(got, "user") {
		t.Fatalf("credentials leaked from sanitized output: %q", got)
	}
}

func TestGitSanitizationRedactsQueriesAuthorizationAndLimitsUTF8(t *testing.T) {
	args := sanitizedArgs([]string{
		"-c", "http.extraHeader=Authorization: Bearer secret-token",
		"clone", "https://example.com/repo.git?access_token=query-secret&safe=value",
	})
	if strings.Contains(args, "secret-token") || strings.Contains(args, "query-secret") || strings.Contains(args, "safe=value") {
		t.Fatalf("sensitive git args leaked: %q", args)
	}
	output := sanitizeOutput("fatal Authorization: Basic abc123 " + "https://example.com/repo?token=xyz " + strings.Repeat("界", 2000))
	if strings.Contains(output, "abc123") || strings.Contains(output, "xyz") {
		t.Fatalf("sensitive git output leaked: %q", output)
	}
	if len([]byte(output)) > 1024 || !utf8.ValidString(output) {
		t.Fatalf("output is not bounded valid UTF-8: bytes=%d", len([]byte(output)))
	}
	diagnostic := gitDiagnosticError([]string{"clone", "https://example.com/repo?token=args-secret"}, errors.New("exit status 1"), "Authorization: Bearer output-secret "+strings.Repeat("界", 2000)).Error()
	if strings.Contains(diagnostic, "args-secret") || strings.Contains(diagnostic, "output-secret") || len([]byte(diagnostic)) > 1024 || !utf8.ValidString(diagnostic) {
		t.Fatalf("unsafe final diagnostic: bytes=%d value=%q", len([]byte(diagnostic)), diagnostic)
	}
}

func TestPersistentCacheDirectoriesArePrivateAndExistingModesTightened(t *testing.T) {
	cache := t.TempDir()
	repos := filepath.Join(cache, "repos")
	if err := os.Mkdir(repos, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{resolved: strings.Repeat("a", 40)}
	svc := Service{LibraryRoot: t.TempDir(), CacheRoot: cache, Runner: runner}
	items, err := svc.AddGit(context.Background(), "acme/repo", GitAddOptions{SourcePath: "packages/skill"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("unexpected items: %#v", items)
	}
	repo, err := RepoCachePath(cache, "acme/repo")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{repos, repo} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("cache directory %q mode=%v", path, info.Mode())
		}
	}
	if err := os.Chmod(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateRef(context.Background(), items[0], nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(repo)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("existing mirror mode not tightened: %v %v", info, err)
	}
}
