package link_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func makeSkill(t *testing.T, root, id string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(id))
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "SKILL.md"), []byte(id), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInspectUsesLexicalOwnershipForBrokenLinks(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	target := filepath.Join(root, "ide", "skills", "demo")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(library, "local", "demo")
	if err := os.Symlink(inside, target); err != nil {
		t.Fatal(err)
	}
	s, err := link.Inspect(target, library)
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind != link.StateManagedLink || !s.Broken || s.SkillID != "local/demo" {
		t.Fatalf("broken internal link not recognized lexically: %#v", s)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "elsewhere", "demo"), target); err != nil {
		t.Fatal(err)
	}
	s, err = link.Inspect(target, library)
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind != link.StateExternalLink || !s.Broken {
		t.Fatalf("external broken link misclassified: %#v", s)
	}
}

func TestPlanAndExecuteOrdinaryMatrix(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	home := filepath.Join(root, "home")
	dir := filepath.Join(home, ".cursor", "skills")
	one := makeSkill(t, library, "local/one")
	makeSkill(t, library, "local/two")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/two"), filepath.Join(dir, "wrong")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/two"), filepath.Join(dir, "orphan")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "blocked"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(dir, "external")); err != nil {
		t.Fatal(err)
	}

	target := link.Target{Scope: config.Scope{Agent: "cursor"}, Home: home, Dir: dir, Desired: map[string]string{
		"missing": "local/one", "correct": "local/one", "wrong": "local/one",
		"blocked": "local/one", "external": "local/one", "lost": "local/lost",
	}}
	if err := os.Symlink(one, filepath.Join(dir, "correct")); err != nil {
		t.Fatal(err)
	}
	p := link.BuildPlan(library, []link.Target{target}, nil, link.Selector{})
	if !hasAction(p, link.ActionCreate, filepath.Join(dir, "missing")) || !hasAction(p, link.ActionReplace, filepath.Join(dir, "wrong")) || !hasAction(p, link.ActionRemove, filepath.Join(dir, "orphan")) {
		t.Fatalf("ordinary actions missing: %#v", p)
	}
	if !hasIssue(p, link.IssueConflict, filepath.Join(dir, "blocked")) || !hasIssue(p, link.IssueConflict, filepath.Join(dir, "external")) || !hasIssue(p, link.IssueLibraryMissing, filepath.Join(dir, "lost")) {
		t.Fatalf("ordinary issues missing: %#v", p)
	}
	res := link.Execute(p, false)
	if len(res.Failures) != 0 {
		t.Fatalf("execute failures: %#v", res.Failures)
	}
	for _, name := range []string{"missing", "correct", "wrong"} {
		s, err := link.Inspect(filepath.Join(dir, name), library)
		if err != nil {
			t.Fatal(err)
		}
		if s.Kind != link.StateManagedLink || s.SkillID != "local/one" {
			t.Fatalf("%s: %#v", name, s)
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, "orphan")); !os.IsNotExist(err) {
		t.Fatalf("orphan not removed: %v", err)
	}
}

func TestDryRunAndScopeFilteringAreReadOnly(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/x")
	home := filepath.Join(root, "home")
	gdir := filepath.Join(home, ".cursor", "skills")
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	pdir := filepath.Join(projectRoot, ".cursor", "skills")
	targets := []link.Target{
		{Scope: config.Scope{Agent: "cursor"}, Home: home, Dir: gdir, Desired: map[string]string{"x": "local/x"}},
		{Scope: config.Scope{Agent: "cursor", Project: "p", ProjectPath: projectRoot}, Dir: pdir, Desired: map[string]string{"x": "local/x"}},
	}
	p := link.BuildPlan(library, targets, nil, link.Selector{Agent: "cursor"})
	if len(p.Actions) != 1 || p.Actions[0].Path != filepath.Join(gdir, "x") {
		t.Fatalf("scope leaked: %#v", p.Actions)
	}
	link.Execute(p, true)
	if _, err := os.Lstat(filepath.Join(gdir, "x")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote disk: %v", err)
	}
}

func TestProjectMissingReturnsWarning(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/x")
	missing := filepath.Join(root, "missing")
	a, _ := agent.ByName("cursor")
	p := link.BuildPlan(library, []link.Target{{Scope: config.Scope{Project: "p", ProjectPath: missing, Agent: "cursor"}, Dir: a.ProjectSkillDir(missing), Desired: map[string]string{"x": "local/x"}}}, nil, link.Selector{})
	if len(p.Actions) != 0 || len(p.Warnings) != 1 || p.Warnings[0].Kind != link.IssueProjectMissing {
		t.Fatalf("missing project not warned: %#v", p)
	}
}

func TestBlockedTargetAndUnsafeScopeNeverTouchDisk(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/x")
	dir := filepath.Join(root, "elsewhere")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	managed := filepath.Join(dir, "old")
	if err := os.Symlink(filepath.Join(library, "local/x"), managed); err != nil {
		t.Fatal(err)
	}
	p := link.BuildPlan(library, []link.Target{{Scope: config.Scope{Agent: "cursor"}, Home: filepath.Join(root, "home"), Dir: dir, Desired: map[string]string{}, Blocked: "collision"}}, nil, link.Selector{})
	if len(p.Actions) != 0 || len(p.Issues) != 1 {
		t.Fatalf("blocked target planned writes: %#v", p)
	}
	if _, err := os.Lstat(managed); err != nil {
		t.Fatalf("blocked target changed: %v", err)
	}
	p = link.BuildPlan(library, []link.Target{{Scope: config.Scope{Agent: "cursor"}, Home: filepath.Join(root, "home"), Dir: dir, Desired: map[string]string{"x": "local/x"}}}, nil, link.Selector{})
	if len(p.Actions) != 0 || !hasIssue(p, link.IssueUnsafePath, dir) {
		t.Fatalf("unsafe scope accepted: %#v", p)
	}
}

func TestBrokenAndRealFileMatrix(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/x")
	dir := filepath.Join(root, "home", ".cursor", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/missing"), filepath.Join(dir, "broken-wrong")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/orphan"), filepath.Join(dir, "broken-orphan")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(dir, "broken-external")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("user"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := link.BuildPlan(library, []link.Target{{Scope: config.Scope{Agent: "cursor"}, Home: filepath.Join(root, "home"), Dir: dir, Desired: map[string]string{"broken-wrong": "local/x", "broken-external": "local/x", "file": "local/x"}}}, nil, link.Selector{})
	if !hasAction(p, link.ActionReplace, filepath.Join(dir, "broken-wrong")) || !hasAction(p, link.ActionRemove, filepath.Join(dir, "broken-orphan")) {
		t.Fatalf("broken internal matrix missing: %#v", p)
	}
	if !hasIssue(p, link.IssueConflict, filepath.Join(dir, "broken-external")) || !hasIssue(p, link.IssueConflict, filepath.Join(dir, "file")) {
		t.Fatalf("external/file conflicts missing: %#v", p)
	}
}

func hasAction(p link.Plan, kind link.ActionKind, path string) bool {
	for _, a := range p.Actions {
		if a.Kind == kind && a.Path == path {
			return true
		}
	}
	return false
}
func hasIssue(p link.Plan, kind link.IssueKind, path string) bool {
	for _, i := range p.Issues {
		if i.Kind == kind && i.Path == path {
			return true
		}
	}
	return false
}

func TestSelectorMatchesExactlySpecifiedScopes(t *testing.T) {
	g := config.Scope{Agent: "cursor"}
	pCursor := config.Scope{Project: "p", ProjectPath: "/p", Agent: "cursor"}
	pCodex := config.Scope{Project: "p", ProjectPath: "/p", Agent: "codex"}
	q := config.Scope{Project: "q", ProjectPath: "/q", Agent: "cursor"}
	cases := []struct {
		s    link.Selector
		want []bool
	}{
		{link.Selector{}, []bool{true, true, true, true}},
		{link.Selector{Agent: "cursor"}, []bool{true, false, false, false}},
		{link.Selector{Project: "p"}, []bool{false, true, true, false}},
		{link.Selector{Project: "p", Agent: "cursor"}, []bool{false, true, false, false}},
	}
	for _, tc := range cases {
		got := []bool{tc.s.Matches(g), tc.s.Matches(pCursor), tc.s.Matches(pCodex), tc.s.Matches(q)}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("selector %#v index %d: got %v want %v", tc.s, i, got, tc.want)
			}
		}
	}
}

func TestReplaceDoesNotClobberObjectAppearingAtTombstone(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/old")
	makeSkill(t, library, "local/new")
	home := filepath.Join(root, "home")
	dir := filepath.Join(home, ".cursor", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "x")
	if err := os.Symlink(filepath.Join(library, "local/old"), target); err != nil {
		t.Fatal(err)
	}
	p := link.BuildPlan(library, []link.Target{{Scope: config.Scope{Agent: "cursor"}, Home: home, Dir: dir, Desired: map[string]string{"x": "local/new"}}}, nil, link.Selector{})
	ops := link.FileOps{Symlink: os.Symlink, MoveNoReplace: func(from, to string) error {
		if err := os.WriteFile(to, []byte("user"), 0o644); err != nil {
			return err
		}
		return os.ErrExist
	}, Remove: os.Remove}
	r := link.ExecuteWithOps(p, false, ops)
	if len(r.Failures) != 1 {
		t.Fatalf("collision not reported: %#v", r)
	}
	s, err := link.Inspect(target, library)
	if err != nil || s.Kind != link.StateManagedLink || s.SkillID != "local/old" {
		t.Fatalf("original clobbered: %#v %v", s, err)
	}
}

func TestExecuteContinuesAfterOneTargetFails(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/x")
	home := filepath.Join(root, "home")
	dir := filepath.Join(home, ".cursor", "skills")
	p := link.BuildPlan(library, []link.Target{{Scope: config.Scope{Agent: "cursor"}, Home: home, Dir: dir, Desired: map[string]string{"a": "local/x", "b": "local/x"}}}, nil, link.Selector{})
	calls := 0
	ops := link.FileOps{Symlink: func(old, new string) error {
		calls++
		if calls == 1 {
			return errors.New("first denied")
		}
		return os.Symlink(old, new)
	}, MoveNoReplace: moveNoReplaceForTest, Remove: os.Remove}
	r := link.ExecuteWithOps(p, false, ops)
	if len(r.Failures) != 1 || len(r.Applied) != 1 {
		t.Fatalf("partial result not preserved: %#v", r)
	}
	if _, err := os.Lstat(filepath.Join(dir, "b")); err != nil {
		t.Fatalf("second target not executed: %v", err)
	}
}

func TestLibraryEntrySymlinkEscapeIsMissingAndNeverLinked(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	if err := os.MkdirAll(filepath.Join(library, "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(library, "local", "x")); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	dir := filepath.Join(home, ".cursor", "skills")
	p := link.BuildPlan(library, []link.Target{{Scope: config.Scope{Agent: "cursor"}, Home: home, Dir: dir, Desired: map[string]string{"x": "local/x"}}}, nil, link.Selector{})
	if len(p.Actions) != 0 || !hasIssue(p, link.IssueLibraryMissing, filepath.Join(dir, "x")) {
		t.Fatalf("escaping library entry accepted: %#v", p)
	}
}

func TestOverlayIntermediateSymlinkEscapeRejectedByPlanAndExecute(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/x")
	home := filepath.Join(root, "home")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, ".cursor")); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".cursor", "skills")
	p := link.BuildPlan(library, []link.Target{{Scope: config.Scope{Agent: "cursor"}, Home: home, Dir: dir, Desired: map[string]string{"x": "local/x"}}}, nil, link.Selector{})
	if len(p.Actions) != 0 || !hasIssue(p, link.IssueUnsafePath, dir) {
		t.Fatalf("overlay escape planned: %#v", p)
	}
	a := link.Action{Kind: link.ActionCreate, Scope: config.Scope{Agent: "cursor"}, Home: home, TargetDir: dir, Path: filepath.Join(dir, "x"), SkillID: "local/x", Library: library}
	r := link.Execute(link.Plan{Actions: []link.Action{a}}, false)
	if len(r.Failures) != 1 {
		t.Fatalf("crafted action escaped: %#v", r)
	}
	if _, err := os.Lstat(filepath.Join(outside, "skills", "x")); !os.IsNotExist(err) {
		t.Fatalf("wrote outside overlay: %v", err)
	}
}
