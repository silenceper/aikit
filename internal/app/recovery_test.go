package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
	"gopkg.in/yaml.v3"
)

func TestResumeRejectsInvalidRollbackSourceBeforeRecover(t *testing.T) {
	application, paths, _, projectPath := testApp(t)
	target := filepath.Join(projectPath, ".cursor", "skills", "demo")
	fingerprint := &config.Fingerprint{Kind: "symlink", Hash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", LinkTarget: filepath.Join(paths.LibrarySkills, "local", "demo")}
	child := config.PendingOperation{
		ID: "cleanup-child", Kind: config.OperationCleanup,
		Scope: config.Scope{Project: "demo", ProjectPath: projectPath, Agent: "cursor"}, Target: target, SkillID: "local/demo",
		TransactionID: "tx-invalid", TransactionPhase: config.TransactionRollback,
		ExpectedSkillID: "local/demo", Expected: fingerprint, Tombstone: filepath.Join(filepath.Dir(target), ".aikit-cleanup-cleanup-child"),
	}
	source := config.PendingOperation{
		ID: "reconcile-source", Kind: config.OperationReconcile, Scope: child.Scope, Target: target, SkillID: "local/demo",
		TransactionID: "tx-invalid", TransactionPhase: config.TransactionRollbackSource, ExpectedAbsent: true,
		Tombstone: filepath.Join(filepath.Dir(target), ".aikit-reconcile-reconcile-source"), Rollback: &child,
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately omit the required top-level child. Save cannot be used
	// because validation must reject these bytes on the next load.
	cfg.PendingOperations = []config.PendingOperation{source}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Config, data, 0o600); err != nil {
		t.Fatal(err)
	}
	recoverCalls := 0
	application.deps.Recover = func(string, []config.PendingOperation, link.Selector, bool) link.Result {
		recoverCalls++
		return link.Result{}
	}
	if _, err := application.ResumeRecovery(context.Background(), RecoveryRequest{Confirmed: true}); err == nil {
		t.Fatal("invalid rollback-source config was loaded")
	}
	if recoverCalls != 0 {
		t.Fatalf("Recover ran %d times for invalid config", recoverCalls)
	}
}

func TestRecoveryPreviewIsReadOnlyAndResumeIsExplicit(t *testing.T) {
	application, paths, userHome, projectPath := testApp(t)
	target := filepath.Join(userHome, ".cursor", "skills", "demo")
	op, err := link.NewCleanupOperation("cleanup-demo", config.Scope{Agent: "cursor"}, target, "local/demo", "test")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.PendingOperations = []config.PendingOperation{op}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, paths.Home, userHome, projectPath)
	preview, err := application.PreviewRecovery(context.Background(), RecoveryRequest{OperationIDs: []string{op.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Operations) != 1 || !preview.Operations[0].CanResume || !preview.Operations[0].CanRollback {
		t.Fatalf("recovery preview: %+v", preview)
	}
	if after := snapshotTree(t, paths.Home, userHome, projectPath); !reflect.DeepEqual(before, after) {
		t.Fatalf("recovery preview wrote state\nbefore=%v\nafter=%v", before, after)
	}
	if _, err := application.ResumeRecovery(context.Background(), RecoveryRequest{OperationIDs: []string{op.ID}}); err == nil {
		t.Fatal("unconfirmed resume accepted")
	}
	result, err := application.ResumeRecovery(context.Background(), RecoveryRequest{OperationIDs: []string{op.ID}, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || len(result.Completed) != 1 {
		t.Fatalf("resume result: %+v", result)
	}
}

func TestRecoveryRollbackReportsUnsupportedAdoptState(t *testing.T) {
	application, paths, _, projectPath := testApp(t)
	target := filepath.Join(projectPath, ".cursor", "skills", "source")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "unknown.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	op, err := link.NewAdoptOperation("adopt-demo", config.Scope{Project: "demo", ProjectPath: projectPath, Agent: "cursor"}, target, "local/demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(target, op.Backup); err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.PendingOperations = []config.PendingOperation{op}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	preview, err := application.PreviewRecovery(context.Background(), RecoveryRequest{OperationIDs: []string{op.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Operations) != 1 || preview.Operations[0].CanRollback || preview.Operations[0].RollbackUnavailable == "" {
		t.Fatalf("unsupported rollback was not reported: %+v", preview)
	}
	if _, err := application.RollbackRecovery(context.Background(), RecoveryRequest{OperationIDs: []string{op.ID}, Confirmed: true}); err == nil {
		t.Fatal("unsupported rollback was accepted")
	}
	if got, err := os.ReadFile(filepath.Join(op.Backup, "unknown.txt")); err != nil || string(got) != "keep" {
		t.Fatalf("unsupported rollback changed unknown content: %q %v", got, err)
	}
}

func TestRecoveryRollbackRejectsPristineAdoptWithoutChangingContent(t *testing.T) {
	application, paths, _, projectPath := testApp(t)
	target := filepath.Join(projectPath, ".cursor", "skills", "source")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "unknown.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	op, err := link.NewAdoptOperation("adopt-pristine", config.Scope{Project: "demo", ProjectPath: projectPath, Agent: "cursor"}, target, "local/demo")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.PendingOperations = []config.PendingOperation{op}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	preview, err := application.PreviewRecovery(context.Background(), RecoveryRequest{OperationIDs: []string{op.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Operations) != 1 || preview.Operations[0].CanRollback || preview.Operations[0].RollbackUnavailable == "" {
		t.Fatalf("unsafe pristine adopt rollback was offered: %+v", preview)
	}
	if _, err := application.RollbackRecovery(context.Background(), RecoveryRequest{OperationIDs: []string{op.ID}, Confirmed: true}); err == nil {
		t.Fatal("unsafe pristine adopt rollback was accepted")
	}
	if got, err := os.ReadFile(filepath.Join(target, "unknown.txt")); err != nil || string(got) != "keep" {
		t.Fatalf("rollback changed original content: %q %v", got, err)
	}
}

func TestOrdinaryMutationsAreGatedByPendingRecovery(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	op, err := link.NewCleanupOperation("pending", config.Scope{Agent: "cursor"}, filepath.Join(userHome, ".cursor", "skills", "demo"), "local/demo", "test")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.PendingOperations = []config.PendingOperation{op}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	_, err = application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: "cursor"})
	var pending *PendingRecoveryError
	if !errors.As(err, &pending) || len(pending.Operations) != 1 {
		t.Fatalf("ordinary mutation error = %T %v", err, err)
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.PendingOperations) != 1 || len(cfg.Agents["cursor"].Skills) != 0 {
		t.Fatalf("gated mutation changed config: %+v", cfg)
	}
}

func TestForwardTransactionRollbackIsUnavailableAndKeepsGate(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	op, err := link.NewCleanupOperation("forward-cleanup", config.Scope{Agent: "cursor"}, filepath.Join(userHome, ".cursor", "skills", "demo"), "local/demo", "batch disable forward")
	if err != nil {
		t.Fatal(err)
	}
	op.TransactionID = "tx-forward"
	op.TransactionPhase = config.TransactionForward
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.PendingOperations = []config.PendingOperation{op}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	preview, err := application.PreviewRecovery(context.Background(), RecoveryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Operations) != 1 || preview.Operations[0].CanRollback || preview.Operations[0].RollbackUnavailable == "" {
		t.Fatalf("forward transaction offered unsafe rollback: %+v", preview)
	}
	if _, err := application.RollbackRecovery(context.Background(), RecoveryRequest{Confirmed: true}); err == nil {
		t.Fatal("forward transaction rollback was accepted")
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.PendingOperations) != 1 {
		t.Fatalf("failed rollback removed forward recovery gate: %+v", cfg.PendingOperations)
	}
}

func TestInvalidRecoveryAuthorizationDoesNotCallLibraryRecovery(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	ops := make([]config.PendingOperation, 2)
	for i, agent := range []string{"cursor", "codex"} {
		var err error
		ops[i], err = link.NewCleanupOperation("group-"+agent, config.Scope{Agent: agent}, filepath.Join(userHome, "."+agent, "skills", "demo"), "local/demo", "forward")
		if err != nil {
			t.Fatal(err)
		}
		ops[i].TransactionID = "tx-group"
		ops[i].TransactionPhase = config.TransactionForward
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.PendingOperations = ops
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	recovery := &recordingRecovery{}
	application.deps.LibraryRecovery = recovery
	if _, err := application.ResumeRecovery(context.Background(), RecoveryRequest{OperationIDs: []string{ops[0].ID}, Confirmed: true}); err == nil {
		t.Fatal("partial transaction resume accepted")
	}
	if _, err := application.RollbackRecovery(context.Background(), RecoveryRequest{Confirmed: true}); err == nil {
		t.Fatal("forward transaction rollback accepted")
	}
	if len(recovery.ledgers) != 0 {
		t.Fatalf("unauthorized recovery called library recovery: %+v", recovery.ledgers)
	}
}

func TestCrashedBatchDisableForwardCannotBeCancelled(t *testing.T) {
	application, paths, _, _ := testApp(t)
	for _, agent := range []string{"cursor", "codex"} {
		if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: agent}); err != nil {
			t.Fatal(err)
		}
	}
	application.deps.Recover = func(string, []config.PendingOperation, link.Selector, bool) link.Result {
		panic("crash after forward checkpoint")
	}
	func() {
		defer func() { _ = recover() }()
		_, _ = application.Batch(context.Background(), BatchRequest{Operation: BatchDisable, Bindings: []BindingRequest{{SkillID: "local/demo", Agent: "cursor"}, {SkillID: "local/demo", Agent: "codex"}}, Confirmed: true})
	}()
	application.deps.Recover = link.Recover
	preview, err := application.PreviewRecovery(context.Background(), RecoveryRequest{})
	if err != nil || len(preview.Operations) != 2 {
		t.Fatalf("preview crashed disable: %+v %v", preview, err)
	}
	for _, operation := range preview.Operations {
		if operation.CanRollback || operation.Operation.TransactionPhase != config.TransactionForward {
			t.Fatalf("crashed disable offered rollback: %+v", operation)
		}
	}
	if _, err := application.RollbackRecovery(context.Background(), RecoveryRequest{Confirmed: true}); err == nil {
		t.Fatal("crashed disable rollback was accepted")
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil || len(cfg.PendingOperations) != 2 {
		t.Fatalf("crashed disable gate was removed: %+v %v", cfg, err)
	}
}

func TestCrashedBatchRemoveForwardCannotBeCancelled(t *testing.T) {
	application, paths, _, _ := testApp(t)
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	application.deps.Library = &fakeLibrary{root: paths.LibrarySkills, commitPanic: true}
	func() {
		defer func() { _ = recover() }()
		_, _ = application.Batch(context.Background(), BatchRequest{Operation: BatchRemove, SkillIDs: []string{"local/demo"}, Force: true, Confirmed: true})
	}()
	preview, err := application.PreviewRecovery(context.Background(), RecoveryRequest{})
	if err != nil || len(preview.Operations) != 1 {
		t.Fatalf("preview crashed remove: %+v %v", preview, err)
	}
	if preview.Operations[0].CanRollback || preview.Operations[0].Operation.TransactionPhase != config.TransactionForward {
		t.Fatalf("crashed remove offered rollback: %+v", preview.Operations[0])
	}
	if _, err := application.RollbackRecovery(context.Background(), RecoveryRequest{Confirmed: true}); err == nil {
		t.Fatal("crashed remove rollback was accepted")
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil || len(cfg.PendingOperations) != 1 {
		t.Fatalf("crashed remove gate was removed: %+v %v", cfg, err)
	}
}
