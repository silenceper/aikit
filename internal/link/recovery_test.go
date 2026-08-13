package link_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func adoptOperation(t *testing.T, root string) (config.PendingOperation, string) {
	t.Helper()
	target := filepath.Join(root, ".cursor", "skills", "demo")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	libSkill := filepath.Join(root, "library", "skills", "local", "demo")
	if _, err := os.Stat(libSkill); err == nil {
		if err := os.WriteFile(filepath.Join(libSkill, "SKILL.md"), []byte("original"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fp, err := link.FingerprintPath(target)
	if err != nil {
		t.Fatal(err)
	}
	journalHash, err := link.DeleteJournalHash(target, fp)
	if err != nil {
		t.Fatal(err)
	}
	return config.PendingOperation{
		ID: "op", Kind: config.OperationAdopt, Scope: config.Scope{Project: "p", ProjectPath: root, Agent: "cursor"}, Target: target, SkillID: "local/demo",
		Temp:   filepath.Join(filepath.Dir(target), ".aikit-adopt-temp-nonce"),
		Backup: filepath.Join(filepath.Dir(target), ".aikit-adopt-backup-nonce"), Original: &fp, JournalHash: journalHash,
	}, target
}

func TestCleanupOnlyDeletesExpectedManagedLink(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	target := filepath.Join(root, "project", ".cursor", "skills", "demo")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/demo"), target); err != nil {
		t.Fatal(err)
	}
	op := config.PendingOperation{ID: "clean", Kind: config.OperationCleanup, Scope: config.Scope{Project: "p", ProjectPath: filepath.Join(root, "project"), Agent: "cursor"}, Target: target, SkillID: "local/demo"}
	r := link.Recover(library, []config.PendingOperation{op}, link.Selector{Project: "p", Agent: "cursor"}, false)
	if len(r.Completed) != 1 {
		t.Fatalf("cleanup not completed: %#v", r)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target remains: %v", err)
	}
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	r = link.Recover(library, []config.PendingOperation{op}, link.Selector{Project: "p", Agent: "cursor"}, false)
	if len(r.Completed) != 0 || len(r.Issues) != 1 || r.Issues[0].Kind != link.IssuePendingCleanup {
		t.Fatalf("later user content was accepted: %#v", r)
	}
	if st, err := os.Stat(target); err != nil || !st.IsDir() {
		t.Fatalf("user directory deleted: %v", err)
	}
}

func TestAdoptRecoveryMatrix(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(t *testing.T, op config.PendingOperation, target, library string)
	}{
		{"original-target", func(t *testing.T, op config.PendingOperation, target, library string) {}},
		{"backup-and-temp", func(t *testing.T, op config.PendingOperation, target, library string) {
			if err := os.Rename(target, op.Backup); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(library, "local/demo"), op.Temp); err != nil {
				t.Fatal(err)
			}
		}},
		{"backup-no-temp", func(t *testing.T, op config.PendingOperation, target, library string) {
			if err := os.Rename(target, op.Backup); err != nil {
				t.Fatal(err)
			}
		}},
		{"target-link-backup", func(t *testing.T, op config.PendingOperation, target, library string) {
			if err := os.Rename(target, op.Backup); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(library, "local/demo"), target); err != nil {
				t.Fatal(err)
			}
		}},
		{"completed-link", func(t *testing.T, op config.PendingOperation, target, library string) {
			if err := os.RemoveAll(target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(library, "local/demo"), target); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			library := filepath.Join(root, "library", "skills")
			makeSkill(t, library, "local/demo")
			op, target := adoptOperation(t, root)
			tc.arrange(t, op, target, library)
			r := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
			if len(r.Completed) != 1 || len(r.Issues) != 0 {
				t.Fatalf("not recovered: %#v", r)
			}
			s, err := link.Inspect(target, library)
			if err != nil {
				t.Fatal(err)
			}
			if s.Kind != link.StateManagedLink || s.SkillID != "local/demo" {
				t.Fatalf("bad target: %#v", s)
			}
			if _, err := os.Lstat(op.Backup); !os.IsNotExist(err) {
				t.Fatalf("backup remains: %v", err)
			}
		})
	}
}

func TestAdoptFingerprintMismatchNeverDeletes(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	op, target := adoptOperation(t, root)
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
	if len(r.Completed) != 0 || len(r.Issues) != 1 || r.Issues[0].Kind != link.IssueAdoptRecovery {
		t.Fatalf("mismatch not reported: %#v", r)
	}
	b, err := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if err != nil || string(b) != "changed" {
		t.Fatalf("changed data lost: %q %v", b, err)
	}
}

func TestAdoptLibraryCopyMismatchPreservesBackup(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	op, target := adoptOperation(t, root)
	if err := os.Rename(target, op.Backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/demo"), target); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(library, "local/demo", "SKILL.md"), []byte("not-the-copy"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
	if len(r.Completed) != 0 || len(r.Issues) != 1 || r.Issues[0].Kind != link.IssueAdoptRecovery {
		t.Fatalf("mismatch not retained: %#v", r)
	}
	match, err := link.FingerprintPath(op.Backup)
	if err != nil || match.Hash != op.Original.Hash {
		t.Fatalf("backup changed: %#v %v", match, err)
	}
}

func TestAdoptDryRunAndScopeDoNotAdvance(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	op, target := adoptOperation(t, root)
	r := link.Recover(library, []config.PendingOperation{op}, link.Selector{Agent: "codex"}, false)
	if len(r.Actions) != 0 {
		t.Fatalf("unrelated scope planned: %#v", r)
	}
	r = link.Recover(library, []config.PendingOperation{op}, link.Selector{}, true)
	if len(r.Completed) != 0 || len(r.Actions) == 0 {
		t.Fatalf("bad dry-run result: %#v", r)
	}
	if st, err := os.Stat(target); err != nil || !st.IsDir() {
		t.Fatalf("dry-run changed target: %v", err)
	}
}

func TestAdoptOperationalFailuresRemainRecoverable(t *testing.T) {
	t.Run("temp creation failure", func(t *testing.T) {
		root := t.TempDir()
		library := filepath.Join(root, "library", "skills")
		makeSkill(t, library, "local/demo")
		op, target := adoptOperation(t, root)
		ops := link.FileOps{Symlink: func(string, string) error { return errors.New("no symlink") }, MoveNoReplace: moveNoReplaceForTest, Remove: os.Remove}
		r := link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, ops)
		if len(r.Completed) != 0 || len(r.Issues) != 1 {
			t.Fatalf("failure was not retained: %#v", r)
		}
		if st, err := os.Stat(target); err != nil || !st.IsDir() {
			t.Fatalf("original changed: %v", err)
		}
	})

	t.Run("first rename failure removes temp", func(t *testing.T) {
		root := t.TempDir()
		library := filepath.Join(root, "library", "skills")
		makeSkill(t, library, "local/demo")
		op, target := adoptOperation(t, root)
		ops := link.FileOps{Symlink: os.Symlink, MoveNoReplace: func(string, string) error { return errors.New("rename denied") }, Remove: os.Remove}
		r := link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, ops)
		if len(r.Completed) != 0 || len(r.Issues) != 1 {
			t.Fatalf("failure was not retained: %#v", r)
		}
		if st, err := os.Stat(target); err != nil || !st.IsDir() {
			t.Fatalf("original changed: %v", err)
		}
		if _, err := os.Lstat(op.Temp); !os.IsNotExist(err) {
			t.Fatalf("temp was not removed: %v", err)
		}
	})

	t.Run("install failure rollback succeeds and retry completes", func(t *testing.T) {
		root := t.TempDir()
		library := filepath.Join(root, "library", "skills")
		makeSkill(t, library, "local/demo")
		op, target := adoptOperation(t, root)
		calls := 0
		ops := link.FileOps{Symlink: os.Symlink, MoveNoReplace: func(old, new string) error {
			calls++
			if calls == 2 {
				return errors.New("install denied")
			}
			return os.Rename(old, new)
		}, Remove: os.Remove}
		r := link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, ops)
		if len(r.Completed) != 0 || len(r.Issues) != 1 {
			t.Fatalf("failure was not retained: %#v", r)
		}
		if st, err := os.Stat(target); err != nil || !st.IsDir() {
			t.Fatalf("rollback did not restore original: %v", err)
		}
		r = link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
		if len(r.Completed) != 1 || len(r.Issues) != 0 {
			t.Fatalf("retry failed: %#v", r)
		}
	})

	t.Run("failed rollback preserves backup", func(t *testing.T) {
		root := t.TempDir()
		library := filepath.Join(root, "library", "skills")
		makeSkill(t, library, "local/demo")
		op, target := adoptOperation(t, root)
		calls := 0
		ops := link.FileOps{Symlink: os.Symlink, MoveNoReplace: func(old, new string) error {
			calls++
			if calls >= 2 {
				return errors.New("rename denied")
			}
			return os.Rename(old, new)
		}, Remove: os.Remove}
		r := link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, ops)
		if len(r.Completed) != 0 || len(r.Issues) != 1 {
			t.Fatalf("rollback failure was not retained: %#v", r)
		}
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("target should reflect interruption: %v", err)
		}
		match, err := link.FingerprintPath(op.Backup)
		if err != nil || match.Hash != op.Original.Hash {
			t.Fatalf("backup was lost or changed: %#v %v", match, err)
		}
	})
}

func moveNoReplaceForTest(from, to string) error {
	if _, err := os.Lstat(to); err == nil {
		return os.ErrExist
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(from, to)
}

func TestNewAdoptOperationUsesReservedSiblingNamesAndProjectPath(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, ".cursor", "skills", "demo")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := config.Scope{Project: "p", ProjectPath: root, Agent: "cursor"}
	op, err := link.NewAdoptOperation("", s, target, "local/demo")
	if err != nil {
		t.Fatal(err)
	}
	if op.Scope.ProjectPath != root || filepath.Dir(op.Temp) != filepath.Dir(target) || filepath.Dir(op.Backup) != filepath.Dir(target) {
		t.Fatalf("wrong operation paths: %#v", op)
	}
	if len(op.JournalHash) != 64 {
		t.Fatalf("journal digest missing: %#v", op)
	}
	if filepath.Base(op.Temp)[:18] != ".aikit-adopt-temp-" || filepath.Base(op.Backup)[:20] != ".aikit-adopt-backup-" {
		t.Fatalf("wrong reserved names: %#v", op)
	}
}

func TestCleanupRecoveryRejectsEveryWrongObject(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	makeSkill(t, library, "local/other")
	base := filepath.Join(root, "project")
	dir := filepath.Join(base, ".cursor", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		setup func(string) error
	}{{"wrong-managed", func(p string) error { return os.Symlink(filepath.Join(library, "local/other"), p) }}, {"external", func(p string) error { return os.Symlink(filepath.Join(root, "outside"), p) }}, {"directory", func(p string) error { return os.Mkdir(p, 0o755) }}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := filepath.Join(dir, tc.name)
			if err := tc.setup(target); err != nil {
				t.Fatal(err)
			}
			op := config.PendingOperation{ID: tc.name, Kind: config.OperationCleanup, Scope: config.Scope{Project: "p", ProjectPath: base, Agent: "cursor"}, Target: target, SkillID: "local/demo"}
			r := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
			if len(r.Completed) != 0 || len(r.Issues) != 1 {
				t.Fatalf("wrong object accepted: %#v", r)
			}
			if _, err := os.Lstat(target); err != nil {
				t.Fatalf("wrong object removed: %v", err)
			}
		})
	}
	target := filepath.Join(dir, "absent")
	op := config.PendingOperation{ID: "absent", Kind: config.OperationCleanup, Scope: config.Scope{Project: "p", ProjectPath: base, Agent: "cursor"}, Target: target, SkillID: "local/demo"}
	r := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
	if len(r.Completed) != 1 {
		t.Fatalf("absent cleanup not completed: %#v", r)
	}
}

func TestAdoptSymlinkLateErrorCleansOnlyExpectedTemp(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	op, _ := adoptOperation(t, root)
	ops := link.FileOps{Symlink: func(old, new string) error {
		if err := os.Symlink(old, new); err != nil {
			return err
		}
		return errors.New("late error")
	}, MoveNoReplace: moveNoReplaceForTest, Remove: os.Remove}
	r := link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, ops)
	if len(r.Issues) != 1 {
		t.Fatalf("late error hidden: %#v", r)
	}
	if _, err := os.Lstat(op.Temp); !os.IsNotExist(err) {
		t.Fatalf("partial temp remains: %v", err)
	}
}

func TestAdoptBackupDestinationRaceNeverClobbers(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	op, target := adoptOperation(t, root)
	calls := 0
	ops := link.FileOps{Symlink: os.Symlink, MoveNoReplace: func(from, to string) error {
		calls++
		if calls == 1 {
			if err := os.WriteFile(to, []byte("user"), 0o644); err != nil {
				return err
			}
			return os.ErrExist
		}
		return moveNoReplaceForTest(from, to)
	}, Remove: os.Remove}
	r := link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, ops)
	if len(r.Issues) != 1 {
		t.Fatalf("race hidden: %#v", r)
	}
	if st, err := os.Stat(target); err != nil || !st.IsDir() {
		t.Fatalf("target changed: %v", err)
	}
	b, err := os.ReadFile(op.Backup)
	if err != nil || string(b) != "user" {
		t.Fatalf("racing content clobbered: %q %v", b, err)
	}
}

func TestAdoptSourceChangeDuringMoveIsRestoredNotInstalled(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	op, target := adoptOperation(t, root)
	calls := 0
	ops := link.FileOps{Symlink: os.Symlink, MoveNoReplace: func(from, to string) error {
		calls++
		if err := moveNoReplaceForTest(from, to); err != nil {
			return err
		}
		if calls == 1 {
			return os.WriteFile(filepath.Join(to, "SKILL.md"), []byte("changed during move"), 0o644)
		}
		return nil
	}, Remove: os.Remove}
	r := link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, ops)
	if len(r.Issues) != 1 || len(r.Completed) != 0 {
		t.Fatalf("source race hidden: %#v", r)
	}
	b, err := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if err != nil || string(b) != "changed during move" {
		t.Fatalf("changed user data lost: %q %v", b, err)
	}
	if _, err := os.Lstat(op.Temp); !os.IsNotExist(err) {
		t.Fatalf("temp remains after restored source: %v", err)
	}
}

func TestAdoptFingerprintIgnoresGitDirectory(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	op, target := adoptOperation(t, root)
	if err := os.MkdirAll(filepath.Join(target, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, ".git", "config"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp, err := link.FingerprintPath(target)
	if err != nil {
		t.Fatal(err)
	}
	op.Original = &fp
	op.JournalHash, err = link.DeleteJournalHash(target, fp)
	if err != nil {
		t.Fatal(err)
	}
	r := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
	if len(r.Completed) != 1 || len(r.Issues) != 0 {
		t.Fatalf(".git prevented adopt: %#v", r)
	}
}

func TestAdoptWrongTempIsUntouched(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	op, target := adoptOperation(t, root)
	if err := os.WriteFile(op.Temp, []byte("user temp"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
	if len(r.Issues) != 1 || len(r.Completed) != 0 {
		t.Fatalf("wrong temp accepted: %#v", r)
	}
	b, _ := os.ReadFile(op.Temp)
	if string(b) != "user temp" {
		t.Fatalf("wrong temp changed")
	}
	if st, err := os.Stat(target); err != nil || !st.IsDir() {
		t.Fatalf("target changed: %v", err)
	}
}

func TestExternalSymlinkFingerprintIncludesTargetAndContent(t *testing.T) {
	root := t.TempDir()
	content := filepath.Join(root, "content")
	if err := os.MkdirAll(content, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(content, "SKILL.md"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "link")
	if err := os.Symlink("content", path); err != nil {
		t.Fatal(err)
	}
	fp, err := link.FingerprintPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if fp.Kind != "symlink" || fp.LinkTarget != "content" || fp.Hash == "" {
		t.Fatalf("incomplete symlink fingerprint: %#v", fp)
	}
	if err := os.WriteFile(filepath.Join(content, "SKILL.md"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := link.FingerprintPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Hash == fp.Hash {
		t.Fatalf("target content change was not fingerprinted")
	}
}

func TestBackupDeleteFailureRemainsRecoverableInReservedTombstone(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	op, target := adoptOperation(t, root)
	if err := os.Rename(target, op.Backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/demo"), target); err != nil {
		t.Fatal(err)
	}
	ops := link.FileOps{Symlink: os.Symlink, MoveNoReplace: moveNoReplaceForTest, Remove: func(path string) error {
		if strings.Contains(filepath.Base(path), ".aikit-adopt-entry-") {
			return errors.New("delete denied")
		}
		return os.Remove(path)
	}}
	r := link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, ops)
	if len(r.Completed) != 0 || len(r.Issues) != 1 {
		t.Fatalf("delete failure hidden: %#v", r)
	}
	r = link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
	if len(r.Completed) != 1 || len(r.Issues) != 0 {
		t.Fatalf("tombstone recovery failed: %#v", r)
	}
}

func TestSafeTreeDeletionPreservesConcurrentAddition(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	op, target := adoptOperation(t, root)
	if err := os.Rename(target, op.Backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/demo"), target); err != nil {
		t.Fatal(err)
	}
	added := false
	ops := link.FileOps{Symlink: os.Symlink, MoveNoReplace: moveNoReplaceForTest, Remove: func(path string) error {
		if strings.Contains(filepath.Base(path), ".aikit-adopt-entry-") && !added {
			added = true
			deleteRoot := filepath.Join(filepath.Dir(op.Backup), ".aikit-adopt-delete-op")
			if err := os.WriteFile(filepath.Join(deleteRoot, "new-user-file"), []byte("keep"), 0o644); err != nil {
				return err
			}
		}
		return os.Remove(path)
	}}
	r := link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, ops)
	if len(r.Completed) != 0 || len(r.Issues) != 1 {
		t.Fatalf("concurrent addition hidden: %#v", r)
	}
	found := false
	entries, _ := os.ReadDir(filepath.Dir(op.Backup))
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".aikit-adopt-entry-") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(filepath.Dir(op.Backup), entry.Name(), "new-user-file"))
		if err == nil && string(b) == "keep" {
			found = true
		}
	}
	if !found {
		t.Fatalf("concurrent file was not retained")
	}
}

func TestSafeTreeDeletionPreservesLeafReplacedDuringQuarantine(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	op, target := adoptOperation(t, root)
	if err := os.Rename(target, op.Backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/demo"), target); err != nil {
		t.Fatal(err)
	}
	calls := 0
	ops := link.FileOps{Symlink: os.Symlink, MoveNoReplace: func(from, to string) error {
		if err := moveNoReplaceForTest(from, to); err != nil {
			return err
		}
		if strings.Contains(filepath.Base(to), ".aikit-adopt-entry-") {
			calls++
			if calls == 1 {
				return os.WriteFile(to, []byte("replacement"), 0o644)
			}
		}
		return nil
	}, Remove: os.Remove}
	r := link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, ops)
	if len(r.Completed) != 0 || len(r.Issues) != 1 {
		t.Fatalf("replacement hidden: %#v", r)
	}
	found := false
	entries, _ := os.ReadDir(filepath.Dir(op.Backup))
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".aikit-adopt-entry-") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(filepath.Dir(op.Backup), entry.Name()))
		if err == nil && string(b) == "replacement" {
			found = true
		}
	}
	if !found {
		t.Fatalf("replacement was not retained")
	}
}
