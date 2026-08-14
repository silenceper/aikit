package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func TestBatchRemoveDeletesAllSelectedSkillsAtomically(t *testing.T) {
	application, paths, _, _ := testApp(t)
	other := config.Skill{ID: "local/other", Name: "other", Hash: "other"}
	otherRoot := filepath.Join(paths.LibrarySkills, "local", "other")
	if err := os.MkdirAll(otherRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherRoot, "SKILL.md"), []byte("---\nname: other\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Library.Skills = append(cfg.Library.Skills, other)
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	result, err := application.Batch(context.Background(), BatchRequest{Operation: BatchRemove, SkillIDs: []string{"local/demo", other.ID}, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || len(result.Items) != 2 {
		t.Fatalf("remove result: %+v", result)
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Library.Skills) != 0 || len(cfg.PendingOperations) != 0 {
		t.Fatalf("remove ledger: %+v", cfg)
	}
	for _, id := range []string{"local/demo", other.ID} {
		if _, err := os.Stat(filepath.Join(paths.LibrarySkills, filepath.FromSlash(id))); !os.IsNotExist(err) {
			t.Fatalf("library skill %s survived: %v", id, err)
		}
	}
}

func TestBatchRemovePartialCleanupRetainsLibraryLedgerForExplicitRetry(t *testing.T) {
	application, paths, _, _ := testApp(t)
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	application.deps.Recover = func(_ string, operations []config.PendingOperation, _ link.Selector, _ bool) link.Result {
		return link.Result{Issues: []link.Issue{{Kind: link.IssuePendingCleanup, Operation: operations[0].ID, Path: operations[0].Target, SkillID: operations[0].SkillID, Message: "changed concurrently"}}}
	}
	_, err := application.Batch(context.Background(), BatchRequest{Operation: BatchRemove, SkillIDs: []string{"local/demo"}, Force: true, Confirmed: true})
	var pending *PendingRecoveryError
	if !errors.As(err, &pending) || len(pending.Operations) == 0 {
		t.Fatalf("partial cleanup did not leave explicit rollback recovery: %T %v", err, err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if skillIndex(cfg, "local/demo") < 0 || len(cfg.PendingOperations) != 2 {
		t.Fatalf("partial remove lost retry ledger: %+v", cfg)
	}
	if cfg.PendingOperations[0].TransactionPhase != config.TransactionRollbackSource || cfg.PendingOperations[1].ParentOperationID != cfg.PendingOperations[0].ID {
		t.Fatalf("partial remove lost forward artifact ownership: %+v", cfg.PendingOperations)
	}
	if got := cfg.Agents["cursor"].Skills; len(got) != 1 || got[0] != "local/demo" {
		t.Fatalf("partial cleanup pruned original references: %+v", cfg.Agents)
	}
	if _, err := os.Stat(filepath.Join(paths.LibrarySkills, "local", "demo")); err != nil {
		t.Fatalf("partial remove changed library content: %v", err)
	}
}

func TestBatchRemovePartialCleanupRollsBackEveryLinkAndReference(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	for _, agent := range []string{"cursor", "codex"} {
		if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: agent}); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	application.deps.Recover = func(libraryRoot string, operations []config.PendingOperation, selector link.Selector, dryRun bool) link.Result {
		calls++
		if calls == 1 {
			applied := link.Recover(libraryRoot, operations[:1], selector, dryRun)
			applied.Actions = append(applied.Actions, link.Action{Kind: link.ActionRecover, Path: operations[1].Target, SkillID: operations[1].SkillID, Operation: operations[1].ID})
			applied.Issues = append(applied.Issues, link.Issue{Kind: link.IssuePendingCleanup, Operation: operations[1].ID, Path: operations[1].Target, SkillID: operations[1].SkillID, Message: "injected second cleanup failure"})
			return applied
		}
		return link.Recover(libraryRoot, operations, selector, dryRun)
	}
	if _, err := application.Batch(context.Background(), BatchRequest{Operation: BatchRemove, SkillIDs: []string{"local/demo"}, Force: true, Confirmed: true}); err == nil {
		t.Fatal("partial cleanup was reported as successful remove")
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if skillIndex(cfg, "local/demo") < 0 || len(cfg.PendingOperations) != 0 || len(cfg.Agents["cursor"].Skills) != 1 || len(cfg.Agents["codex"].Skills) != 1 {
		t.Fatalf("remove rollback did not restore full config: %+v", cfg)
	}
	for _, path := range []string{filepath.Join(userHome, ".cursor", "skills", "demo"), filepath.Join(userHome, ".codex", "skills", "demo")} {
		assertLink(t, path)
	}
	if _, err := os.Stat(filepath.Join(paths.LibrarySkills, "local", "demo")); err != nil {
		t.Fatalf("remove rollback changed library: %v", err)
	}
}

func TestBatchRemoveCrashAfterFirstCleanupResumesDurableForwardState(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	for _, agent := range []string{"cursor", "codex"} {
		if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: agent}); err != nil {
			t.Fatal(err)
		}
	}
	application.deps.Recover = func(libraryRoot string, operations []config.PendingOperation, selector link.Selector, dryRun bool) link.Result {
		if len(operations) < 2 || operations[0].Kind != config.OperationCleanup {
			return link.Recover(libraryRoot, operations, selector, dryRun)
		}
		_ = link.Recover(libraryRoot, operations[:1], selector, dryRun)
		panic("injected crash after first cleanup")
	}
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("injected cleanup crash did not occur")
			}
		}()
		_, _ = application.Batch(context.Background(), BatchRequest{Operation: BatchRemove, SkillIDs: []string{"local/demo"}, Force: true, Confirmed: true})
	}()
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if skillIndex(cfg, "local/demo") >= 0 || len(cfg.Agents["cursor"].Skills) != 0 || len(cfg.Agents["codex"].Skills) != 0 || len(cfg.PendingOperations) != 2 {
		t.Fatalf("remove crash did not retain final ledger plus forward and rollback intent: %+v", cfg)
	}
	for _, operation := range cfg.PendingOperations {
		if operation.TransactionPhase != config.TransactionForward || operation.Rollback == nil || operation.Rollback.TransactionPhase != config.TransactionRollback {
			t.Fatalf("remove forward/rollback operation is incomplete: %+v", operation)
		}
	}
	application.deps.Recover = link.Recover
	if _, err := application.Batch(context.Background(), BatchRequest{Operation: BatchDisable, Bindings: []BindingRequest{{SkillID: "local/demo", Agent: "cursor"}}, Confirmed: true}); err == nil {
		t.Fatal("ordinary mutation bypassed remove crash recovery gate")
	}
	if _, err := application.ResumeRecovery(context.Background(), RecoveryRequest{Confirmed: true}); err != nil {
		t.Fatalf("resume remove rollback: %v", err)
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.PendingOperations) != 0 || skillIndex(cfg, "local/demo") >= 0 {
		t.Fatalf("remove resume did not converge durable forward state: %+v", cfg)
	}
	for _, path := range []string{filepath.Join(userHome, ".cursor", "skills", "demo"), filepath.Join(userHome, ".codex", "skills", "demo")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("remove resume left target %s: %v", path, err)
		}
	}
}

func TestBatchRemovePrepareFailureLeavesOldConfigUntouched(t *testing.T) {
	application, paths, _, _ := testApp(t)
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	application.deps.Library = &fakeLibrary{root: paths.LibrarySkills, removeBatchPanic: true}
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("injected prepare crash did not occur")
			}
		}()
		_, _ = application.Batch(context.Background(), BatchRequest{Operation: BatchRemove, SkillIDs: []string{"local/demo"}, Force: true, Confirmed: true})
	}()
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if skillIndex(cfg, "local/demo") < 0 || len(cfg.Agents["cursor"].Skills) != 1 || len(cfg.PendingOperations) != 0 {
		t.Fatalf("failed library prepare changed old config: %+v", cfg)
	}
}

func TestBatchRemoveCrashAfterPrepareBeforeCheckpointKeepsOldDirection(t *testing.T) {
	application, paths, _, _ := testApp(t)
	application.deps.Library = &fakeLibrary{root: paths.LibrarySkills}
	application.deps.AfterRemovePrepare = func() { panic("crash after prepare") }
	func() {
		defer func() { _ = recover() }()
		_, _ = application.Batch(context.Background(), BatchRequest{Operation: BatchRemove, SkillIDs: []string{"local/demo"}, Confirmed: true})
	}()
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil || skillIndex(cfg, "local/demo") < 0 || len(cfg.PendingOperations) != 0 {
		t.Fatalf("prepare-before-checkpoint crash changed durable direction: %+v %v", cfg, err)
	}
}

func TestBatchRemoveCrashAfterCheckpointResumesForwardDirection(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	application.deps.Library = &fakeLibrary{root: paths.LibrarySkills}
	application.deps.AfterRemoveCheckpoint = func() { panic("crash after checkpoint") }
	func() {
		defer func() { _ = recover() }()
		_, _ = application.Batch(context.Background(), BatchRequest{Operation: BatchRemove, SkillIDs: []string{"local/demo"}, Force: true, Confirmed: true})
	}()
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil || skillIndex(cfg, "local/demo") >= 0 || len(cfg.PendingOperations) != 1 {
		t.Fatalf("checkpoint crash lost forward direction: %+v %v", cfg, err)
	}
	application.deps.AfterRemoveCheckpoint = nil
	application.deps.LibraryRecovery = recoveryFunc(func(_ context.Context, ledger []config.Skill) ([]library.RecoveryIssue, error) {
		if len(ledger) == 0 {
			return nil, os.RemoveAll(filepath.Join(paths.LibrarySkills, "local", "demo"))
		}
		return nil, nil
	})
	if _, err := application.ResumeRecovery(context.Background(), RecoveryRequest{Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil || len(cfg.PendingOperations) != 0 || skillIndex(cfg, "local/demo") >= 0 {
		t.Fatalf("forward resume did not converge ledger: %+v %v", cfg, err)
	}
	if _, err := os.Lstat(filepath.Join(userHome, ".cursor", "skills", "demo")); !os.IsNotExist(err) {
		t.Fatalf("forward resume left link: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.LibrarySkills, "local", "demo")); !os.IsNotExist(err) {
		t.Fatalf("forward resume left library: %v", err)
	}
}

func TestBatchRemoveCommitFailureRetainsEverySelectedLibraryLedgerEntry(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	other := config.Skill{ID: "local/other", Name: "other", Hash: "other"}
	otherRoot := filepath.Join(paths.LibrarySkills, "local", "other")
	if err := os.MkdirAll(otherRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherRoot, "SKILL.md"), []byte("---\nname: other\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Library.Skills = append(cfg.Library.Skills, other)
	cfg.Agents = map[string]config.Binding{"cursor": {Skills: []string{"local/demo", other.ID}}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]string{"demo": "local/demo", "other": other.ID} {
		path := filepath.Join(userHome, ".cursor", "skills", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(paths.LibrarySkills, filepath.FromSlash(id)), path); err != nil {
			t.Fatal(err)
		}
	}
	fake := &fakeLibrary{root: paths.LibrarySkills, commitErr: errors.New("commit failed")}
	recovery := &recordingRecovery{}
	application.deps.Library = fake
	application.deps.LibraryRecovery = recovery
	if _, err := application.Batch(context.Background(), BatchRequest{Operation: BatchRemove, SkillIDs: []string{"local/demo", other.ID}, Force: true, Confirmed: true}); err == nil {
		t.Fatal("commit failure was lost")
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if skillIndex(cfg, "local/demo") < 0 || skillIndex(cfg, other.ID) < 0 {
		t.Fatalf("batch remove failure lost ledger entries: %+v", cfg.Library.Skills)
	}
	if got := cfg.Agents["cursor"].Skills; len(got) != 2 {
		t.Fatalf("batch remove failure lost references: %v", got)
	}
	for _, name := range []string{"demo", "other"} {
		assertLink(t, filepath.Join(userHome, ".cursor", "skills", name))
	}
	last := recovery.ledgers[len(recovery.ledgers)-1]
	if len(last) != 2 {
		t.Fatalf("batch remove recovery used incomplete ledger: %+v", last)
	}
}

func TestBatchUpdateFailureRestoresOldLedgerAndUsesOldRecoveryLedger(t *testing.T) {
	aikitHome, userHome := t.TempDir(), t.TempDir()
	t.Setenv("AIKIT_HOME", aikitHome)
	t.Setenv("HOME", userHome)
	paths := config.PathsForHome(aikitHome)
	old := []config.Skill{
		{ID: "github.com/acme/repo/one", Name: "one", Source: "github.com/acme/repo", SourcePath: "one", Ref: &config.Ref{Kind: "branch", Value: "main"}, Resolved: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{ID: "github.com/acme/repo/two", Name: "two", Source: "github.com/acme/repo", SourcePath: "two", Ref: &config.Ref{Kind: "branch", Value: "main"}, Resolved: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	cfg := config.New()
	cfg.Library.Skills = append([]config.Skill(nil), old...)
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	fake := &fakeLibrary{root: paths.LibrarySkills, commitErr: errors.New("commit failed")}
	recovery := &recordingRecovery{}
	application := New(Dependencies{Store: config.Store{Paths: paths}, Paths: paths, UserHome: userHome, Library: fake, LibraryRecovery: recovery})
	expected := map[string]ExpectedUpdate{}
	for _, skill := range old {
		expected[skill.ID] = ExpectedUpdate{Ref: skill.Ref, Resolved: skill.Resolved, Remote: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	}
	if _, err := application.Batch(context.Background(), BatchRequest{Operation: BatchUpdate, SkillIDs: []string{old[0].ID, old[1].ID}, Expected: expected, Confirmed: true}); err == nil {
		t.Fatal("commit failure was lost")
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Library.Skills, old) {
		t.Fatalf("old update ledger was not restored: %+v", cfg.Library.Skills)
	}
	last := recovery.ledgers[len(recovery.ledgers)-1]
	if !reflect.DeepEqual(last, old) {
		t.Fatalf("recovery used new ledger: %+v", last)
	}
}

func TestBatchCancellationCausesZeroWrites(t *testing.T) {
	application, paths, userHome, projectPath := testApp(t)
	before := snapshotTree(t, paths.Home, userHome, projectPath)
	if _, err := application.Batch(context.Background(), BatchRequest{Operation: BatchEnable, Bindings: []BindingRequest{{SkillID: "local/demo", Agent: "cursor"}}}); err == nil {
		t.Fatal("unconfirmed batch accepted")
	}
	if after := snapshotTree(t, paths.Home, userHome, projectPath); !reflect.DeepEqual(before, after) {
		t.Fatalf("cancelled batch changed state\nbefore=%v\nafter=%v", before, after)
	}
}

func TestBatchEnableIsOneAtomicPreflightedMutation(t *testing.T) {
	application, paths, userHome, projectPath := testApp(t)
	conflict := filepath.Join(projectPath, ".cursor", "skills", "demo")
	if err := os.MkdirAll(conflict, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conflict, "unknown.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Lock acquisition itself creates this stable coordination file; exclude
	// that one-time setup from the mutation snapshot.
	if err := os.WriteFile(paths.Lock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, paths.Home, userHome, projectPath)

	_, err := application.Batch(context.Background(), BatchRequest{
		Operation: BatchEnable,
		Bindings: []BindingRequest{
			{SkillID: "local/demo", Agent: "codex"},
			{SkillID: "local/demo", Project: "demo"},
		},
		Confirmed: true,
	})
	if err == nil {
		t.Fatal("batch conflict was accepted")
	}
	after := snapshotTree(t, paths.Home, userHome, projectPath)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("conflicted batch changed durable state\nbefore=%v\nafter=%v", before, after)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Agents["codex"].Skills) != 0 || len(cfg.Projects[0].Skills) != 0 {
		t.Fatalf("conflicted batch changed bindings: %+v", cfg)
	}
}

func TestBatchDisableAppliesAllBindingsWithOneRecoveryExecution(t *testing.T) {
	application, paths, _, _ := testApp(t)
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Agents = map[string]config.Binding{"cursor": {Skills: []string{"local/demo"}}}
	if cfg.Projects[0].AgentBindings == nil {
		cfg.Projects[0].AgentBindings = map[string]config.Binding{}
	}
	cfg.Projects[0].AgentBindings["cursor"] = config.Binding{Skills: []string{"local/demo"}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	executions := 0
	application.deps.Recover = func(root string, operations []config.PendingOperation, selector link.Selector, dryRun bool) link.Result {
		if dryRun {
			t.Fatal("batch execution was dry-run")
		}
		executions++
		return link.Recover(root, operations, selector, false)
	}
	result, err := application.Batch(context.Background(), BatchRequest{
		Operation: BatchDisable,
		Bindings: []BindingRequest{
			{SkillID: "local/demo", Agent: "cursor"},
			{SkillID: "local/demo", Project: "demo", Agent: "cursor"},
		},
		Confirmed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if executions != 1 || !result.Changed || len(result.Items) != 2 {
		t.Fatalf("batch result=%+v executions=%d", result, executions)
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Agents["cursor"].Skills) != 0 || len(cfg.Projects[0].AgentBindings["cursor"].Skills) != 0 {
		t.Fatalf("batch did not remove every binding: %+v", cfg)
	}
}

func TestBatchBindingPartialExecutionRollsBackConfigAndAppliedLinks(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	calls := 0
	application.deps.Recover = func(root string, operations []config.PendingOperation, selector link.Selector, dryRun bool) link.Result {
		calls++
		if calls == 1 {
			if len(operations) < 2 {
				t.Fatalf("forward operations need two actions: %+v", operations)
			}
			applied := link.Recover(root, operations[:1], selector, false)
			applied.Issues = append(applied.Issues, link.Issue{Kind: link.IssueIO, Path: operations[1].Target, SkillID: operations[1].SkillID, Message: "injected second action failure"})
			return applied
		}
		return link.Recover(root, operations, selector, dryRun)
	}
	_, err := application.Batch(context.Background(), BatchRequest{
		Operation: BatchEnable,
		Bindings: []BindingRequest{
			{SkillID: "local/demo", Agent: "cursor"},
			{SkillID: "local/demo", Agent: "codex"},
		},
		Confirmed: true,
	})
	if err == nil {
		t.Fatal("partial forward execution was reported as success")
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Agents["cursor"].Skills) != 0 || len(cfg.Agents["codex"].Skills) != 0 || len(cfg.PendingOperations) != 0 {
		t.Fatalf("old binding config was not restored: %+v", cfg)
	}
	for _, path := range []string{filepath.Join(userHome, ".cursor", "skills", "demo"), filepath.Join(userHome, ".codex", "skills", "demo")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("partial link survived rollback at %s: %v", path, err)
		}
	}
}

func TestBatchBindingRollbackFailurePersistsExplicitRecovery(t *testing.T) {
	application, paths, _, _ := testApp(t)
	application.deps.Recover = func(root string, operations []config.PendingOperation, selector link.Selector, _ bool) link.Result {
		if operations[0].TransactionPhase == config.TransactionForward {
			applied := link.Recover(root, operations[:1], selector, false)
			applied.Issues = append(applied.Issues, link.Issue{Kind: link.IssueIO, Path: operations[1].Target, SkillID: operations[1].SkillID, Message: "forward failure"})
			return applied
		}
		return link.Result{Issues: []link.Issue{{Kind: link.IssuePendingCleanup, Operation: operations[0].ID, Path: operations[0].Target, SkillID: operations[0].SkillID, Message: "rollback failure"}}}
	}
	_, err := application.Batch(context.Background(), BatchRequest{
		Operation: BatchEnable,
		Bindings:  []BindingRequest{{SkillID: "local/demo", Agent: "cursor"}, {SkillID: "local/demo", Agent: "codex"}},
		Confirmed: true,
	})
	var pending *PendingRecoveryError
	if !errors.As(err, &pending) || len(pending.Operations) == 0 {
		t.Fatalf("rollback failure did not return typed pending recovery: %T %v", err, err)
	}
	cfg, loadErr := (config.Store{Paths: paths}).Load(context.Background())
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(cfg.PendingOperations) == 0 {
		t.Fatal("rollback failure did not persist recovery operations")
	}
	if cfg.PendingOperations[0].Kind != config.OperationCleanup && cfg.PendingOperations[0].Kind != config.OperationReconcile {
		t.Fatalf("unexpected recovery kind: %+v", cfg.PendingOperations)
	}
	application.deps.Recover = link.Recover
	if _, err := application.ResumeRecovery(context.Background(), RecoveryRequest{Confirmed: true}); err != nil {
		t.Fatalf("explicit recovery could not resume rollback: %v", err)
	}
}

func TestBatchBindingUnknownConcurrentTargetKeepsCompleteRollbackGate(t *testing.T) {
	application, paths, _, _ := testApp(t)
	calls := 0
	var unknown string
	application.deps.Recover = func(root string, operations []config.PendingOperation, selector link.Selector, _ bool) link.Result {
		calls++
		if operations[0].TransactionPhase != config.TransactionForward {
			return link.Recover(root, operations, selector, false)
		}
		if len(operations) != 2 {
			t.Fatalf("forward operations: %+v", operations)
		}
		applied := link.Recover(root, operations[:1], selector, false)
		unknown = operations[1].Target
		if err := os.MkdirAll(unknown, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(unknown, "unknown.txt"), []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		applied.Issues = append(applied.Issues, link.Issue{Kind: link.IssueIO, Path: unknown, SkillID: operations[1].SkillID, Message: "concurrent unknown target"})
		return applied
	}
	_, err := application.Batch(context.Background(), BatchRequest{Operation: BatchEnable, Bindings: []BindingRequest{{SkillID: "local/demo", Agent: "cursor"}, {SkillID: "local/demo", Agent: "codex"}}, Confirmed: true})
	var pending *PendingRecoveryError
	if !errors.As(err, &pending) || len(pending.Operations) == 0 {
		t.Fatalf("unknown rollback target lost pending gate: %T %v", err, err)
	}
	cfg, loadErr := (config.Store{Paths: paths}).Load(context.Background())
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(cfg.Agents["cursor"].Skills) != 0 || len(cfg.Agents["codex"].Skills) != 0 || len(cfg.PendingOperations) == 0 {
		t.Fatalf("old config plus rollback gate not restored: %+v", cfg)
	}
	if got, readErr := os.ReadFile(filepath.Join(unknown, "unknown.txt")); readErr != nil || string(got) != "keep" {
		t.Fatalf("unknown content changed: %q %v", got, readErr)
	}
	preview, previewErr := application.PreviewRecovery(context.Background(), RecoveryRequest{})
	if previewErr != nil || len(preview.Issues) == 0 {
		t.Fatalf("preview omitted unknown rollback issue: %+v %v", preview, previewErr)
	}
	if _, gateErr := application.MutatePreset(context.Background(), PresetMutationRequest{Operation: PresetCreate, Name: "blocked", Confirmed: true}); gateErr == nil {
		t.Fatal("ordinary mutation bypassed unknown rollback gate")
	}
}

func TestBatchBindingCrashAfterFirstActionLeavesDurableForwardRecovery(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	application.deps.Recover = func(root string, operations []config.PendingOperation, selector link.Selector, _ bool) link.Result {
		if len(operations) < 2 {
			t.Fatalf("forward operations need two actions: %+v", operations)
		}
		_ = link.Recover(root, operations[:1], selector, false)
		panic("injected crash after first action")
	}
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("injected crash did not occur")
			}
		}()
		_, _ = application.Batch(context.Background(), BatchRequest{
			Operation: BatchEnable,
			Bindings: []BindingRequest{
				{SkillID: "local/demo", Agent: "cursor"},
				{SkillID: "local/demo", Agent: "codex"},
			},
			Confirmed: true,
		})
	}()
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.PendingOperations) != 2 {
		t.Fatalf("crash checkpoint lacks complete forward intent: %+v", cfg.PendingOperations)
	}
	for _, operation := range cfg.PendingOperations {
		if operation.TransactionID == "" || operation.TransactionPhase != config.TransactionForward {
			t.Fatalf("operation lacks forward transaction direction: %+v", operation)
		}
	}
	application.deps.Recover = link.Recover
	if _, err := application.MutatePreset(context.Background(), PresetMutationRequest{Operation: PresetCreate, Name: "blocked", Confirmed: true}); err == nil {
		t.Fatal("ordinary mutation bypassed crash recovery gate")
	}
	preview, err := application.PreviewRecovery(context.Background(), RecoveryRequest{})
	if err != nil || len(preview.Operations) != 2 {
		t.Fatalf("recovery preview unavailable: %+v %v", preview, err)
	}
	if _, err := application.ResumeRecovery(context.Background(), RecoveryRequest{Confirmed: true}); err != nil {
		t.Fatalf("resume forward transaction: %v", err)
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.PendingOperations) != 0 || len(cfg.Agents["cursor"].Skills) != 1 || len(cfg.Agents["codex"].Skills) != 1 {
		t.Fatalf("resume did not converge durable forward config: %+v", cfg)
	}
	for _, path := range []string{filepath.Join(userHome, ".cursor", "skills", "demo"), filepath.Join(userHome, ".codex", "skills", "demo")} {
		assertLink(t, path)
	}
}

func TestBatchDisableCleanupCrashResumesDeterministicTombstone(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(userHome, ".cursor", "skills", "demo")
	application.deps.Recover = func(root string, operations []config.PendingOperation, selector link.Selector, dryRun bool) link.Result {
		return link.RecoverWithOps(root, operations, selector, dryRun, link.FileOps{AfterCleanupMove: func() { panic("crash after cleanup move") }})
	}
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("injected cleanup crash did not occur")
			}
		}()
		_, _ = application.Batch(context.Background(), BatchRequest{Operation: BatchDisable, Bindings: []BindingRequest{{SkillID: "local/demo", Agent: "cursor"}}, Confirmed: true})
	}()
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil || len(cfg.PendingOperations) != 1 {
		t.Fatalf("cleanup crash intent missing: %+v %v", cfg, err)
	}
	if _, err := os.Lstat(cfg.PendingOperations[0].Tombstone); err != nil {
		t.Fatalf("cleanup tombstone missing: %v", err)
	}
	if random, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".aikit-cleanup-delete-*")); len(random) != 0 {
		t.Fatalf("transaction cleanup used random tombstone: %v", random)
	}
	application.deps.Recover = link.Recover
	if _, err := application.ResumeRecovery(context.Background(), RecoveryRequest{Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil || len(cfg.PendingOperations) != 0 {
		t.Fatalf("cleanup resume did not clear gate: %+v %v", cfg, err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("cleanup resume left target: %v", err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".aikit-cleanup-*")); len(leftovers) != 0 {
		t.Fatalf("cleanup resume left tombstone: %v", leftovers)
	}
}

func TestBatchDisableForwardCleanupFailureRetainsArtifactUntilExplicitRollbackResume(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	application.deps.Recover = func(root string, operations []config.PendingOperation, selector link.Selector, dryRun bool) link.Result {
		calls++
		if calls == 1 {
			return link.RecoverWithOps(root, operations, selector, dryRun, link.FileOps{FailCleanupUnlink: errors.New("injected unlink failure")})
		}
		return link.Result{Issues: []link.Issue{{Kind: link.IssuePendingCleanup, Operation: operations[0].ID, Path: operations[0].Target, Message: "pause rollback"}}}
	}
	_, err := application.Batch(context.Background(), BatchRequest{Operation: BatchDisable, Bindings: []BindingRequest{{SkillID: "local/demo", Agent: "cursor"}}, Confirmed: true})
	var pending *PendingRecoveryError
	if !errors.As(err, &pending) {
		t.Fatalf("forward artifact failure did not retain explicit recovery: %T %v", err, err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil || len(cfg.PendingOperations) != 2 || cfg.PendingOperations[0].TransactionPhase != config.TransactionRollbackSource || cfg.PendingOperations[1].ParentOperationID != cfg.PendingOperations[0].ID {
		t.Fatalf("rollback source/dependency not durable: %+v %v", cfg, err)
	}
	if _, err := os.Lstat(cfg.PendingOperations[0].Tombstone); err != nil {
		t.Fatalf("forward cleanup tombstone lost: %v", err)
	}
	application.deps.Recover = link.Recover
	if _, err := application.ResumeRecovery(context.Background(), RecoveryRequest{Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	state, err := link.Inspect(filepath.Join(userHome, ".cursor", "skills", "demo"), paths.LibrarySkills)
	if err != nil || state.SkillID != "local/demo" {
		t.Fatalf("old cleanup link not restored: %+v %v", state, err)
	}
	cfg, _ = (config.Store{Paths: paths}).Load(context.Background())
	if len(cfg.PendingOperations) != 0 {
		t.Fatalf("rollback recovery gate remains: %+v", cfg.PendingOperations)
	}
}

func TestPresetForwardReconcileFailureRetainsArtifactUntilExplicitRollbackResume(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	newSkill := config.Skill{ID: "local/new-demo", Name: "demo", Hash: "new"}
	if err := os.MkdirAll(filepath.Join(paths.LibrarySkills, "local", "new-demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Library.Skills = append(cfg.Library.Skills, newSkill)
	cfg.Presets = []config.Preset{{Name: "review", Skills: []string{"local/demo"}}}
	cfg.Agents = map[string]config.Binding{"cursor": {Presets: []string{"review"}}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Sync(context.Background(), SyncRequest{Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	application.deps.Recover = func(root string, operations []config.PendingOperation, selector link.Selector, dryRun bool) link.Result {
		calls++
		if calls == 1 {
			return link.RecoverWithOps(root, operations, selector, dryRun, link.FileOps{FailReconcileSymlink: errors.New("injected symlink failure")})
		}
		return link.Result{Issues: []link.Issue{{Kind: link.IssuePendingReconcile, Operation: operations[0].ID, Path: operations[0].Target, Message: "pause rollback"}}}
	}
	_, err = application.MutatePreset(context.Background(), PresetMutationRequest{Operation: PresetEditMembers, Name: "review", Skills: []string{newSkill.ID}, Confirmed: true})
	var pending *PendingRecoveryError
	if !errors.As(err, &pending) {
		t.Fatalf("forward reconcile artifact failure did not retain recovery: %T %v", err, err)
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil || len(cfg.PendingOperations) != 2 || cfg.PendingOperations[0].TransactionPhase != config.TransactionRollbackSource {
		t.Fatalf("reconcile rollback source not durable: %+v %v", cfg, err)
	}
	if _, err := os.Lstat(cfg.PendingOperations[0].Tombstone); err != nil {
		t.Fatalf("forward reconcile tombstone lost: %v", err)
	}
	application.deps.Recover = link.Recover
	if _, err := application.ResumeRecovery(context.Background(), RecoveryRequest{Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	state, err := link.Inspect(filepath.Join(userHome, ".cursor", "skills", "demo"), paths.LibrarySkills)
	if err != nil || state.SkillID != "local/demo" {
		t.Fatalf("old reconcile link not restored: %+v %v", state, err)
	}
	cfg, _ = (config.Store{Paths: paths}).Load(context.Background())
	if len(cfg.PendingOperations) != 0 || len(cfg.Presets[0].Skills) != 1 || cfg.Presets[0].Skills[0] != "local/demo" {
		t.Fatalf("old preset/gate did not converge: %+v", cfg)
	}
}

func TestPresetMutationLifecycleAndApply(t *testing.T) {
	application, paths, _, _ := testApp(t)
	ctx := context.Background()
	requests := []PresetMutationRequest{
		{Operation: PresetCreate, Name: "base", Skills: []string{"local/demo"}, Confirmed: true},
		{Operation: PresetDuplicate, Name: "base", NewName: "copy", Confirmed: true},
		{Operation: PresetRename, Name: "copy", NewName: "renamed", Confirmed: true},
		{Operation: PresetEditMembers, Name: "renamed", Skills: nil, Confirmed: true},
		{Operation: PresetApply, Name: "base", Binding: BindingRequest{Agent: "cursor"}, Confirmed: true},
		{Operation: PresetDelete, Name: "renamed", Confirmed: true},
	}
	for _, request := range requests {
		if _, err := application.MutatePreset(ctx, request); err != nil {
			t.Fatalf("%s: %v", request.Operation, err)
		}
	}
	cfg, err := (config.Store{Paths: paths}).Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Presets) != 1 || cfg.Presets[0].Name != "base" || !contains(cfg.Agents["cursor"].Presets, "base") {
		t.Fatalf("preset lifecycle result: %+v", cfg)
	}
}

func TestPresetMutationPartialExecutionRollsBackConfigAndLinks(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Presets = []config.Preset{{Name: "review"}}
	cfg.Agents = map[string]config.Binding{"cursor": {Presets: []string{"review"}}, "codex": {Presets: []string{"review"}}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	calls := 0
	application.deps.Recover = func(root string, operations []config.PendingOperation, selector link.Selector, dryRun bool) link.Result {
		calls++
		if calls == 1 {
			if len(operations) < 2 {
				t.Fatalf("forward preset operations need two actions: %+v", operations)
			}
			applied := link.Recover(root, operations[:1], selector, false)
			applied.Issues = append(applied.Issues, link.Issue{Kind: link.IssueIO, Path: operations[1].Target, SkillID: operations[1].SkillID, Message: "injected failure"})
			return applied
		}
		return link.Recover(root, operations, selector, dryRun)
	}
	if _, err := application.MutatePreset(context.Background(), PresetMutationRequest{Operation: PresetEditMembers, Name: "review", Skills: []string{"local/demo"}, Confirmed: true}); err == nil {
		t.Fatal("partial preset execution was reported as success")
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Presets[0].Skills) != 0 || len(cfg.PendingOperations) != 0 {
		t.Fatalf("old preset config was not restored: %+v", cfg)
	}
	for _, path := range []string{filepath.Join(userHome, ".cursor", "skills", "demo"), filepath.Join(userHome, ".codex", "skills", "demo")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("preset rollback left link %s: %v", path, err)
		}
	}
}

func TestPresetMutationUnknownConcurrentTargetKeepsRollbackGate(t *testing.T) {
	application, paths, _, _ := testApp(t)
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Presets = []config.Preset{{Name: "review"}}
	cfg.Agents = map[string]config.Binding{"cursor": {Presets: []string{"review"}}, "codex": {Presets: []string{"review"}}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	calls := 0
	application.deps.Recover = func(root string, operations []config.PendingOperation, selector link.Selector, _ bool) link.Result {
		calls++
		if operations[0].TransactionPhase != config.TransactionForward {
			return link.Recover(root, operations, selector, false)
		}
		applied := link.Recover(root, operations[:1], selector, false)
		if err := os.MkdirAll(filepath.Dir(operations[1].Target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(operations[1].Target, []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		applied.Issues = append(applied.Issues, link.Issue{Kind: link.IssueIO, Path: operations[1].Target, SkillID: operations[1].SkillID, Message: "concurrent unknown target"})
		return applied
	}
	_, err = application.MutatePreset(context.Background(), PresetMutationRequest{Operation: PresetEditMembers, Name: "review", Skills: []string{"local/demo"}, Confirmed: true})
	var pending *PendingRecoveryError
	if !errors.As(err, &pending) || len(pending.Operations) == 0 {
		t.Fatalf("preset rollback lost unknown target gate: %T %v", err, err)
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Presets[0].Skills) != 0 || len(cfg.PendingOperations) == 0 {
		t.Fatalf("preset old config plus gate not restored: %+v", cfg)
	}
	preview, err := application.PreviewRecovery(context.Background(), RecoveryRequest{})
	if err != nil || len(preview.Issues) == 0 {
		t.Fatalf("preset preview omitted unknown issue: %+v %v", preview, err)
	}
}

func TestPresetForwardReplaceCrashResumesDeterministicRecovery(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	newSkill := config.Skill{ID: "local/new-demo", Name: "demo", Hash: "new"}
	newRoot := filepath.Join(paths.LibrarySkills, filepath.FromSlash(newSkill.ID))
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newRoot, "SKILL.md"), []byte("---\nname: demo\n---\nnew"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Library.Skills = append(cfg.Library.Skills, newSkill)
	cfg.Presets = []config.Preset{{Name: "review", Skills: []string{"local/demo"}}}
	cfg.Agents = map[string]config.Binding{"cursor": {Presets: []string{"review"}}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Sync(context.Background(), SyncRequest{Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(userHome, ".cursor", "skills", "demo")
	application.deps.Recover = func(root string, operations []config.PendingOperation, selector link.Selector, dryRun bool) link.Result {
		return link.RecoverWithOps(root, operations, selector, dryRun, link.FileOps{AfterReconcileMove: func() { panic("crash after deterministic quarantine") }})
	}
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("injected forward reconcile crash did not occur")
			}
		}()
		_, _ = application.MutatePreset(context.Background(), PresetMutationRequest{Operation: PresetEditMembers, Name: "review", Skills: []string{newSkill.ID}, Confirmed: true})
	}()
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil || len(cfg.PendingOperations) != 1 || cfg.PendingOperations[0].TransactionPhase != config.TransactionForward {
		t.Fatalf("forward crash intent missing: %+v %v", cfg, err)
	}
	if _, err := os.Lstat(cfg.PendingOperations[0].Tombstone); err != nil {
		t.Fatalf("deterministic tombstone missing: %v", err)
	}
	if random, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".aikit-link-old-*")); len(random) != 0 {
		t.Fatalf("transactional forward used random execute tombstone: %v", random)
	}
	application.deps.Recover = link.Recover
	if _, err := application.ResumeRecovery(context.Background(), RecoveryRequest{Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	state, err := link.Inspect(target, paths.LibrarySkills)
	if err != nil || state.SkillID != newSkill.ID {
		t.Fatalf("forward resume did not install new skill: %+v %v", state, err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".aikit-reconcile-*")); len(leftovers) != 0 {
		t.Fatalf("completed forward left deterministic tombstone: %v", leftovers)
	}
}

func TestPresetMutationCancellationWritesNothing(t *testing.T) {
	application, paths, userHome, projectPath := testApp(t)
	before := snapshotTree(t, paths.Home, userHome, projectPath)
	if _, err := application.MutatePreset(context.Background(), PresetMutationRequest{Operation: PresetCreate, Name: "nope"}); err == nil {
		t.Fatal("unconfirmed mutation accepted")
	}
	if after := snapshotTree(t, paths.Home, userHome, projectPath); !reflect.DeepEqual(before, after) {
		t.Fatalf("cancel path wrote state\nbefore=%v\nafter=%v", before, after)
	}
}
