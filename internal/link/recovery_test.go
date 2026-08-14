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
	op, err := link.NewCleanupOperation("clean", config.Scope{Project: "p", ProjectPath: filepath.Join(root, "project"), Agent: "cursor"}, target, "local/demo", "test")
	if err != nil {
		t.Fatal(err)
	}
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

func TestRollbackSourceRestoresReconcileAndCleanupTombstones(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/old")
	makeSkill(t, library, "local/new")
	project := filepath.Join(root, "project")
	parent := filepath.Join(project, ".cursor", "skills")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	scope := config.Scope{Project: "p", ProjectPath: project, Agent: "cursor"}

	for _, tc := range []struct {
		name string
		make func(string) (config.PendingOperation, error)
		ops  link.FileOps
	}{
		{name: "reconcile", make: func(target string) (config.PendingOperation, error) {
			return link.NewReconcileOperation("forward-reconcile", scope, target, "local/new", library, "test")
		}, ops: link.FileOps{FailReconcileSymlink: errors.New("injected create failure")}},
		{name: "cleanup", make: func(target string) (config.PendingOperation, error) {
			return link.NewCleanupOperation("forward-cleanup", scope, target, "local/old", "test")
		}, ops: link.FileOps{FailCleanupUnlink: errors.New("injected unlink failure")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := filepath.Join(parent, tc.name)
			if err := os.Symlink(filepath.Join(library, "local/old"), target); err != nil {
				t.Fatal(err)
			}
			op, err := tc.make(target)
			if err != nil {
				t.Fatal(err)
			}
			op.TransactionID = "tx-" + tc.name
			op.TransactionPhase = config.TransactionForward
			failed := link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, tc.ops)
			if len(failed.Issues) != 1 {
				t.Fatalf("forward failure not injected: %#v", failed)
			}
			if _, err := os.Lstat(op.Tombstone); err != nil {
				t.Fatalf("forward tombstone missing: %v", err)
			}
			op.TransactionPhase = config.TransactionRollbackSource
			restored := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
			if len(restored.Completed) != 1 || len(restored.Issues) != 0 {
				t.Fatalf("rollback source did not restore old link: %#v", restored)
			}
			state, err := link.Inspect(target, library)
			if err != nil || state.SkillID != "local/old" {
				t.Fatalf("old link not restored: %+v %v", state, err)
			}
			if _, err := os.Lstat(op.Tombstone); !os.IsNotExist(err) {
				t.Fatalf("forward tombstone remains: %v", err)
			}
		})
	}
}

func TestCleanupDeleteQuarantineRejectsSwappedEntryAndRestoresUnknown(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	project := filepath.Join(root, "project")
	target := filepath.Join(project, ".cursor", "skills", "demo")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/demo"), target); err != nil {
		t.Fatal(err)
	}
	op, err := link.NewCleanupOperation("delete-race", config.Scope{Project: "p", ProjectPath: project, Agent: "cursor"}, target, "local/demo", "test")
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	saved := op.Tombstone + ".saved-authenticated"
	result := link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, link.FileOps{BeforeDeleteQuarantine: func(role string) {
		if role != "tombstone" {
			return
		}
		if err := os.Rename(op.Tombstone, saved); err != nil {
			panic(err)
		}
		if err := os.Symlink(outside, op.Tombstone); err != nil {
			panic(err)
		}
	}})
	if len(result.Completed) != 0 || len(result.Issues) != 1 {
		t.Fatalf("swapped delete entry was accepted: %#v", result)
	}
	if got, err := os.Readlink(op.Tombstone); err != nil || got != outside {
		t.Fatalf("unknown replacement was not restored: %q %v", got, err)
	}
	if state, err := link.Inspect(saved, library); err != nil || state.SkillID != "local/demo" {
		t.Fatalf("authenticated original did not survive race: %+v %v", state, err)
	}
}

func TestCleanupDeleteQuarantineCrashResumes(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	project := filepath.Join(root, "project")
	target := filepath.Join(project, ".cursor", "skills", "demo")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/demo"), target); err != nil {
		t.Fatal(err)
	}
	op, err := link.NewCleanupOperation("delete-crash", config.Scope{Project: "p", ProjectPath: project, Agent: "cursor"}, target, "local/demo", "test")
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() { _ = recover() }()
		link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, link.FileOps{AfterDeleteQuarantine: func(string) { panic("crash") }})
	}()
	quarantine := filepath.Join(filepath.Dir(target), ".aikit-delete-tombstone-"+op.ID)
	if _, err := os.Lstat(quarantine); err != nil {
		t.Fatalf("deterministic delete quarantine missing: %v", err)
	}
	resumed := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
	if len(resumed.Completed) != 1 || len(resumed.Issues) != 0 {
		t.Fatalf("delete quarantine did not resume: %#v", resumed)
	}
	if _, err := os.Lstat(quarantine); !os.IsNotExist(err) {
		t.Fatalf("delete quarantine remains: %v", err)
	}
}

func TestReconcileDeleteQuarantineCrashResumes(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/old")
	makeSkill(t, library, "local/new")
	project := filepath.Join(root, "project")
	target := filepath.Join(project, ".cursor", "skills", "demo")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/old"), target); err != nil {
		t.Fatal(err)
	}
	op, err := link.NewReconcileOperation("reconcile-delete-crash", config.Scope{Project: "p", ProjectPath: project, Agent: "cursor"}, target, "local/new", library, "test")
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() { _ = recover() }()
		link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, link.FileOps{AfterDeleteQuarantine: func(string) { panic("crash") }})
	}()
	resumed := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
	if len(resumed.Completed) != 1 || len(resumed.Issues) != 0 {
		t.Fatalf("reconcile delete quarantine did not resume: %#v", resumed)
	}
	state, err := link.Inspect(target, library)
	if err != nil || state.SkillID != "local/new" {
		t.Fatalf("desired reconcile target missing: %+v %v", state, err)
	}
}

func TestRollbackSourceTargetDeleteQuarantineCrashRestoresOldLink(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/old")
	makeSkill(t, library, "local/new")
	project := filepath.Join(root, "project")
	target := filepath.Join(project, ".cursor", "skills", "demo")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/new"), target); err != nil {
		t.Fatal(err)
	}
	oldFingerprint, err := link.ExpectedManagedFingerprint(library, "local/old")
	if err != nil {
		t.Fatal(err)
	}
	newFingerprint, err := link.ExpectedManagedFingerprint(library, "local/new")
	if err != nil {
		t.Fatal(err)
	}
	op := config.PendingOperation{
		ID: "rollback-target-delete", Kind: config.OperationReconcile,
		Scope: config.Scope{Project: "p", ProjectPath: project, Agent: "cursor"}, Target: target, SkillID: "local/new",
		TransactionID: "tx-delete", TransactionPhase: config.TransactionRollbackSource,
		ExpectedSkillID: "local/old", Expected: oldFingerprint,
		Tombstone: filepath.Join(filepath.Dir(target), ".aikit-reconcile-rollback-target-delete"),
	}
	op.Rollback = &config.PendingOperation{ID: "rollback-child", Kind: config.OperationReconcile, Scope: op.Scope, Target: target, SkillID: "local/old", TransactionID: op.TransactionID, TransactionPhase: config.TransactionRollback, ExpectedSkillID: "local/new", Expected: newFingerprint, Tombstone: filepath.Join(filepath.Dir(target), ".aikit-reconcile-rollback-child")}
	if err := os.Symlink(filepath.Join(library, "local/old"), op.Tombstone); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() { _ = recover() }()
		link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, link.FileOps{AfterDeleteQuarantine: func(role string) {
			if role == "target" {
				panic("crash")
			}
		}})
	}()
	resumed := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
	if len(resumed.Completed) != 1 || len(resumed.Issues) != 0 {
		t.Fatalf("rollback source target delete did not resume: %#v", resumed)
	}
	state, err := link.Inspect(target, library)
	if err != nil || state.SkillID != "local/old" {
		t.Fatalf("old link not restored: %+v %v", state, err)
	}
	for _, pattern := range []string{".aikit-delete-*", ".aikit-reconcile-*"} {
		if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(target), pattern)); len(leftovers) != 0 {
			t.Fatalf("rollback source left artifacts: %v", leftovers)
		}
	}
}

func TestCleanupPreservesConcurrentManagedReplacement(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	makeSkill(t, library, "local/replacement")
	project := filepath.Join(root, "project")
	target := filepath.Join(project, ".cursor", "skills", "demo")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/demo"), target); err != nil {
		t.Fatal(err)
	}
	op, err := link.NewCleanupOperation("cleanup-concurrent", config.Scope{Project: "p", ProjectPath: project, Agent: "cursor"}, target, "local/demo", "disable")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/replacement"), target); err != nil {
		t.Fatal(err)
	}
	r := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
	if len(r.Completed) != 0 || len(r.Issues) != 1 {
		t.Fatalf("concurrent cleanup replacement accepted: %#v", r)
	}
	state, err := link.Inspect(target, library)
	if err != nil || state.SkillID != "local/replacement" {
		t.Fatalf("concurrent replacement changed: %+v %v", state, err)
	}
}

func TestCleanupParentSwapAfterAnchorRetainsPendingAndWritesNothing(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	project := filepath.Join(root, "project")
	parent := filepath.Join(project, ".cursor", "skills")
	target := filepath.Join(parent, "demo")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/demo"), target); err != nil {
		t.Fatal(err)
	}
	op, err := link.NewCleanupOperation("cleanup-parent-race", config.Scope{Project: "p", ProjectPath: project, Agent: "cursor"}, target, "local/demo", "disable")
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(project, ".cursor", "skills-original")
	r := link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, link.FileOps{BeforeCleanupMutation: func() {
		if err := os.Rename(parent, moved); err != nil {
			panic(err)
		}
		if err := os.Symlink(outside, parent); err != nil {
			panic(err)
		}
	}})
	if len(r.Completed) != 0 || len(r.Issues) != 1 || r.Issues[0].Kind != link.IssueUnsafePath {
		t.Fatalf("cleanup parent swap was accepted: %#v", r)
	}
	state, err := link.Inspect(filepath.Join(moved, "demo"), library)
	if err != nil || state.SkillID != "local/demo" {
		t.Fatalf("anchored original target changed: %+v %v", state, err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "demo")); !os.IsNotExist(err) {
		t.Fatalf("cleanup wrote outside swapped parent: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(moved, filepath.Base(op.Tombstone))); !os.IsNotExist(err) {
		t.Fatalf("cleanup created tombstone despite parent swap: %v", err)
	}
}

func TestStandaloneCleanupRollbackRestoresAuthenticatedTombstone(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	project := filepath.Join(root, "project")
	target := filepath.Join(project, ".cursor", "skills", "demo")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/demo"), target); err != nil {
		t.Fatal(err)
	}
	op, err := link.NewCleanupOperation("standalone-crash", config.Scope{Project: "p", ProjectPath: project, Agent: "cursor"}, target, "local/demo", "disable")
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() { _ = recover() }()
		link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, link.FileOps{AfterCleanupMove: func() { panic("crash") }})
	}()
	preview := link.RollbackCleanup(library, []config.PendingOperation{op}, true)
	if len(preview.Completed) != 1 || len(preview.Issues) != 0 {
		t.Fatalf("tombstone rollback unavailable: %#v", preview)
	}
	rolledBack := link.RollbackCleanup(library, []config.PendingOperation{op}, false)
	if len(rolledBack.Completed) != 1 || len(rolledBack.Issues) != 0 {
		t.Fatalf("tombstone rollback failed: %#v", rolledBack)
	}
	state, err := link.Inspect(target, library)
	if err != nil || state.SkillID != "local/demo" {
		t.Fatalf("cleanup link was not restored: %+v %v", state, err)
	}
	if _, err := os.Lstat(op.Tombstone); !os.IsNotExist(err) {
		t.Fatalf("restored cleanup left tombstone: %v", err)
	}
}

func TestStandaloneCleanupRollbackDetectsParentSwapAfterRestoreMove(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	project := filepath.Join(root, "project")
	parent := filepath.Join(project, ".cursor", "skills")
	target := filepath.Join(parent, "demo")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/demo"), target); err != nil {
		t.Fatal(err)
	}
	op, err := link.NewCleanupOperation("rollback-parent-race", config.Scope{Project: "p", ProjectPath: project, Agent: "cursor"}, target, "local/demo", "disable")
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() { _ = recover() }()
		link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, link.FileOps{AfterCleanupMove: func() { panic("crash") }})
	}()
	moved := filepath.Join(project, ".cursor", "skills-original")
	result := link.RollbackCleanupWithOps(library, []config.PendingOperation{op}, false, link.FileOps{AfterCleanupRollbackMove: func() {
		if err := os.Rename(parent, moved); err != nil {
			panic(err)
		}
		if err := os.Symlink(outside, parent); err != nil {
			panic(err)
		}
	}})
	if len(result.Completed) != 0 || len(result.Issues) != 1 || result.Issues[0].Kind != link.IssueUnsafePath {
		t.Fatalf("post-restore parent swap falsely completed: %#v", result)
	}
	state, err := link.Inspect(filepath.Join(moved, "demo"), library)
	if err != nil || state.SkillID != "local/demo" {
		t.Fatalf("restored deterministic state lost: %+v %v", state, err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "demo")); !os.IsNotExist(err) {
		t.Fatalf("rollback wrote outside swapped parent: %v", err)
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
	if err := os.MkdirAll(filepath.Join(root, "outside"), 0o755); err != nil {
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
			op, err := link.NewCleanupOperation(tc.name, config.Scope{Project: "p", ProjectPath: base, Agent: "cursor"}, target, "local/demo", "test")
			if err != nil {
				t.Fatal(err)
			}
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
	op, err := link.NewCleanupOperation("absent", config.Scope{Project: "p", ProjectPath: base, Agent: "cursor"}, target, "local/demo", "test")
	if err != nil {
		t.Fatal(err)
	}
	r := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
	if len(r.Completed) != 1 {
		t.Fatalf("absent cleanup not completed: %#v", r)
	}
}

func TestReconcilePreservesConcurrentManagedReplacement(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	makeSkill(t, library, "local/old")
	makeSkill(t, library, "local/replacement")
	base := filepath.Join(root, "project")
	target := filepath.Join(base, ".cursor", "skills", "demo")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/old"), target); err != nil {
		t.Fatal(err)
	}
	op, err := link.NewReconcileOperation("", config.Scope{Project: "p", ProjectPath: base, Agent: "cursor"}, target, "local/demo", library, "rollback")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/replacement"), target); err != nil {
		t.Fatal(err)
	}
	r := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
	if len(r.Completed) != 0 || len(r.Issues) != 1 || r.Issues[0].Kind != link.IssuePendingReconcile {
		t.Fatalf("concurrent replacement was accepted: %#v", r)
	}
	state, inspectErr := link.Inspect(target, library)
	if inspectErr != nil || state.SkillID != "local/replacement" {
		t.Fatalf("concurrent replacement changed: %#v %v", state, inspectErr)
	}
}

func TestReconcileRejectsParentSymlinkSwap(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	project := filepath.Join(root, "project")
	outside := filepath.Join(root, "outside")
	target := filepath.Join(project, ".cursor", "skills", "demo")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	op, err := link.NewReconcileOperation("", config.Scope{Project: "p", ProjectPath: project, Agent: "cursor"}, target, "local/demo", library, "forward")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(project, ".cursor", "skills")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "skills"), filepath.Join(project, ".cursor", "skills")); err != nil {
		t.Fatal(err)
	}
	r := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
	if len(r.Completed) != 0 || len(r.Issues) != 1 || r.Issues[0].Kind != link.IssueUnsafePath {
		t.Fatalf("parent symlink swap was accepted: %#v", r)
	}
	if _, err := os.Lstat(filepath.Join(outside, "skills", "demo")); !os.IsNotExist(err) {
		t.Fatalf("recovery wrote through swapped parent: %v", err)
	}
}

func TestReconcileResumesAfterCrashImmediatelyAfterQuarantine(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	makeSkill(t, library, "local/old")
	project := filepath.Join(root, "project")
	target := filepath.Join(project, ".cursor", "skills", "demo")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(library, "local/old"), target); err != nil {
		t.Fatal(err)
	}
	op, err := link.NewReconcileOperation("reconcile-crash", config.Scope{Project: "p", ProjectPath: project, Agent: "cursor"}, target, "local/demo", library, "forward")
	if err != nil {
		t.Fatal(err)
	}
	if op.Tombstone == "" || filepath.Dir(op.Tombstone) != filepath.Dir(target) {
		t.Fatalf("deterministic tombstone missing: %+v", op)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("injected quarantine crash did not occur")
			}
		}()
		link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, link.FileOps{AfterReconcileMove: func() { panic("crash") }})
	}()
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target should be absent at crash point: %v", err)
	}
	if _, err := os.Lstat(op.Tombstone); err != nil {
		t.Fatalf("durable tombstone missing after crash: %v", err)
	}
	r := link.Recover(library, []config.PendingOperation{op}, link.Selector{}, false)
	if len(r.Completed) != 1 || len(r.Issues) != 0 {
		t.Fatalf("resume after quarantine crash failed: %#v", r)
	}
	state, err := link.Inspect(target, library)
	if err != nil || state.Kind != link.StateManagedLink || state.SkillID != "local/demo" {
		t.Fatalf("resumed target mismatch: %#v %v", state, err)
	}
	if _, err := os.Lstat(op.Tombstone); !os.IsNotExist(err) {
		t.Fatalf("completed recovery left tombstone: %v", err)
	}
}

func TestReconcileParentSwapAfterAnchorCannotWriteOutside(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library", "skills")
	makeSkill(t, library, "local/demo")
	project := filepath.Join(root, "project")
	parent := filepath.Join(project, ".cursor", "skills")
	target := filepath.Join(parent, "demo")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	op, err := link.NewReconcileOperation("reconcile-race", config.Scope{Project: "p", ProjectPath: project, Agent: "cursor"}, target, "local/demo", library, "forward")
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(project, ".cursor", "skills-original")
	r := link.RecoverWithOps(library, []config.PendingOperation{op}, link.Selector{}, false, link.FileOps{BeforeReconcileMutation: func() {
		if err := os.Rename(parent, moved); err != nil {
			panic(err)
		}
		if err := os.Symlink(outside, parent); err != nil {
			panic(err)
		}
	}})
	if len(r.Completed) != 0 || len(r.Issues) != 1 || r.Issues[0].Kind != link.IssueUnsafePath {
		t.Fatalf("parent swap was accepted: %#v", r)
	}
	if _, err := os.Lstat(filepath.Join(outside, "demo")); !os.IsNotExist(err) {
		t.Fatalf("reconcile wrote outside anchored parent: %v", err)
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
