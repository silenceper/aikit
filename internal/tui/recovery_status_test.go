package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/status"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

func recoveryFixture() app.RecoveryPreview {
	return app.RecoveryPreview{
		Operations: []app.RecoveryOperation{
			{Operation: config.PendingOperation{ID: "op-1", TransactionID: "tx-1", Kind: config.OperationCleanup, Target: "/work/one", Reason: "cleanup"}, CanResume: true},
			{Operation: config.PendingOperation{ID: "op-2", TransactionID: "tx-1", Kind: config.OperationReconcile, Target: "/work/two", Reason: "reconcile"}, CanResume: true},
		},
		Issues: []app.OperationIssue{{Operation: "preview-recovery", Item: "op-2", Path: "/work/two", Message: "target changed", Err: errors.New("fingerprint mismatch")}},
	}
}

func TestStatusRAndMouseRefreshUseSameForcedSnapshotRequest(t *testing.T) {
	run := func(t *testing.T, mouse bool) app.StatusRequest {
		t.Helper()
		service := &fakeService{snapshot: testSnapshot()}
		m := NewModel(nil, service, &fakeMigration{}, ViewStatus, ActionNone)
		m.Snapshot, m.Width, m.Height = testSnapshot(), 100, 24
		var cmd tea.Cmd
		if mouse {
			m, cmd = mouseAction(t, m, actionIndex(t, m, "Refresh"))
		} else {
			next, keyCmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
			m, cmd = next.(Model), keyCmd
		}
		if cmd == nil || !m.Busy || service.snapshotCalls != 0 {
			t.Fatalf("refresh cmd=%v busy=%v calls=%d", cmd != nil, m.Busy, service.snapshotCalls)
		}
		_ = cmd()
		return service.lastSnapshot
	}
	keyboard, mouse := run(t, false), run(t, true)
	if !reflect.DeepEqual(keyboard, mouse) || !keyboard.ForceRefresh || keyboard.Offline {
		t.Fatalf("status refresh mismatch keyboard=%+v mouse=%+v", keyboard, mouse)
	}
	other := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	other.Snapshot = testSnapshot()
	_, cmd := other.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatal("non-status r no longer refreshes local inventory")
	}
}

func TestErrorDetailsUsesSelectedStatusOrUpdateFailureIdentity(t *testing.T) {
	statusMessage := "selected status item full message at /a/very/long/status/path"
	updateError := "selected update check failed with full transport details"
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewStatus, ActionNone)
	m.Width, m.Height = 100, 24
	m.Snapshot = testSnapshot()
	m.Snapshot.Status.Items = []status.Item{{Kind: status.IOError, SkillID: "acme/alpha", Path: "/status/path", Message: statusMessage}}
	m.Snapshot.Updates.Results = append(m.Snapshot.Updates.Results, updatecheck.Result{SkillID: "acme/beta", State: updatecheck.StateCheckFailed, Error: updateError})
	m.Err = "unrelated aggregate error"
	for _, tt := range []struct {
		rowKey, want string
	}{{"status:acme/alpha", statusMessage}, {"status:update-failure:acme/beta", updateError}} {
		t.Run(tt.rowKey, func(t *testing.T) {
			current := m
			for i, row := range current.rows() {
				if row.Key == tt.rowKey {
					current.Cursor = i
					break
				}
			}
			more, _ := keyboardAction(t, current, actionIndex(t, current, "More"))
			index := actionIndex(t, more, "Error details")
			for more.ActionIndex != index {
				next, _ := more.Update(actionKey(tea.KeyRight))
				more = next.(Model)
			}
			next, _ := more.Update(actionKey(tea.KeyEnter))
			detail := next.(Model)
			if detail.FullError != tt.want || !strings.Contains(detail.ViewString(), tt.want) {
				t.Fatalf("selected error detail=%q want=%q\n%s", detail.FullError, tt.want, detail.ViewString())
			}
		})
	}
}

func TestStatusCompositeIdentitySelectsDuplicateSkillExactMessage(t *testing.T) {
	first := status.Item{Kind: status.IOError, Scope: config.Scope{Project: "one", Agent: "codex"}, Path: "/one/path", SkillID: "acme/alpha", Operation: "inspect", Message: "first scope failure"}
	second := status.Item{Kind: status.IOError, Scope: config.Scope{Project: "two", Agent: "codex"}, Path: "/two/path", SkillID: "acme/alpha", Operation: "inspect", Message: "second scope failure"}
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewStatus, ActionNone)
	m.Width, m.Height = 100, 24
	m.Snapshot = testSnapshot()
	m.Snapshot.Status.Items = []status.Item{first, second}
	rows := m.rows()
	if len(rows) != 2 || rows[0].Key == rows[1].Key {
		t.Fatalf("duplicate-skill status rows lack stable unique identity: %+v", rows)
	}
	wantKey := statusItemKey(second)
	for i, row := range rows {
		if row.Key == wantKey {
			m.Cursor = i
		}
	}
	if got := m.selectedErrorDetail(); got != second.Message {
		t.Fatalf("selected second identity returned %q, want %q; rows=%+v", got, second.Message, rows)
	}
}

func TestStartupPendingRecoveryPreviewsExactIDsAndResumeOnly(t *testing.T) {
	service := &fakeService{snapshot: testSnapshot(), recoveryPreview: recoveryFixture(), recoveryResult: app.RecoveryResult{Completed: []string{"op-1", "op-2"}, Changed: true}}
	service.snapshot.Config.PendingOperations = []config.PendingOperation{{ID: "op-1"}, {ID: "op-2"}}
	m := NewModel(nil, service, &fakeMigration{}, ViewOverview, ActionNone)
	next, preview := m.Update(m.Init()())
	m = next.(Model)
	if preview == nil || service.previewRecoveryCalls != 0 || m.Mode == ModeConfirm {
		t.Fatalf("startup recovery preview cmd=%v calls=%d mode=%s", preview != nil, service.previewRecoveryCalls, m.Mode)
	}
	next, _ = m.Update(preview())
	m = next.(Model)
	view := m.ViewString()
	for _, wanted := range []string{"Review recovery", "tx-1", "op-1", "op-2", "/work/one", "/work/two", "Can resume: yes", "target changed", "fingerprint mismatch"} {
		if !strings.Contains(view, wanted) {
			t.Fatalf("recovery preview missing %q:\n%s", wanted, view)
		}
	}
	if strings.Contains(strings.ToLower(view), "rollback") || strings.Contains(strings.Join(m.primaryActions(), " "), "Rollback") {
		t.Fatalf("rollback exposed in recovery UI:\n%s", view)
	}
	cancelled, cancelCmd := m.Update(actionKey(tea.KeyEsc))
	if cancelCmd != nil || service.resumeRecoveryCalls != 0 || cancelled.(Model).Mode == ModeConfirm {
		t.Fatal("recovery cancel mutated")
	}
	next, resume := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if resume == nil || service.resumeRecoveryCalls != 0 || !m.MutationBusy {
		t.Fatal("recovery resume was not deferred")
	}
	_, duplicate := m.Update(actionKey(tea.KeyEnter))
	if duplicate != nil {
		t.Fatal("busy recovery accepted duplicate submit")
	}
	_ = resume()
	want := app.RecoveryRequest{OperationIDs: []string{"op-1", "op-2"}, Confirmed: true}
	if service.resumeRecoveryCalls != 1 || !reflect.DeepEqual(service.lastRecoveryResume, want) {
		t.Fatalf("resume request=%+v want=%+v calls=%d", service.lastRecoveryResume, want, service.resumeRecoveryCalls)
	}
}

func TestPendingRecoveryMutationErrorOpensSameReviewFlow(t *testing.T) {
	service := &fakeService{recoveryPreview: recoveryFixture()}
	m := NewModel(nil, service, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 100, 24
	pending := &app.PendingRecoveryError{Operations: recoveryFixture().Operations}
	next, preview := m.Update(operationMsg{name: "update", err: pending})
	m = next.(Model)
	if preview == nil || service.previewRecoveryCalls != 0 || m.Mode == ModeConfirm {
		t.Fatalf("gate error did not defer preview: cmd=%v calls=%d mode=%s", preview != nil, service.previewRecoveryCalls, m.Mode)
	}
}

func TestRecoveryConfirmKeyboardMouseUseSameExactResumeRequest(t *testing.T) {
	run := func(t *testing.T, mouse bool) app.RecoveryRequest {
		t.Helper()
		service := &fakeService{}
		m := NewModel(nil, service, &fakeMigration{}, ViewOverview, ActionNone)
		m.Width, m.Height = 100, 24
		m.Mode, m.confirm = ModeConfirm, ActionRecovery
		m.RecoveryPreview = recoveryFixture()
		m.pendingRecovery = app.RecoveryRequest{OperationIDs: []string{"op-1", "op-2"}}
		var cmd tea.Cmd
		if mouse {
			region := m.hitRegions().Confirm
			next, mouseCmd := m.Update(click(region.X, region.Y))
			m, cmd = next.(Model), mouseCmd
		} else {
			next, keyCmd := m.Update(actionKey(tea.KeyEnter))
			m, cmd = next.(Model), keyCmd
		}
		if cmd == nil || !m.MutationBusy || service.resumeRecoveryCalls != 0 {
			t.Fatalf("resume cmd=%v busy=%v calls=%d", cmd != nil, m.MutationBusy, service.resumeRecoveryCalls)
		}
		_ = cmd()
		if service.resumeRecoveryCalls != 1 {
			t.Fatalf("resume calls=%d", service.resumeRecoveryCalls)
		}
		return service.lastRecoveryResume
	}
	keyboard := run(t, false)
	mouse := run(t, true)
	if !reflect.DeepEqual(keyboard, mouse) || !keyboard.Confirmed {
		t.Fatalf("keyboard/mouse recovery mismatch: %+v / %+v", keyboard, mouse)
	}
}

func TestStatusRefreshRetryAndFullErrorDetailActions(t *testing.T) {
	service := &fakeService{snapshot: testSnapshot()}
	m := NewModel(nil, service, &fakeMigration{}, ViewStatus, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 100, 24
	m.Snapshot.Status.Items[0].Kind = status.Missing
	m.Err = "compare failed: left hash abcdef does not match right hash fedcba at /very/long/path/to/skill"
	more, _ := keyboardAction(t, m, actionIndex(t, m, "More"))
	for _, label := range []string{"Retry", "Error details"} {
		if actionIndex(t, more, label) < 0 {
			t.Fatal("unreachable")
		}
	}
	idx := actionIndex(t, more, "Error details")
	for more.ActionIndex != idx {
		next, _ := more.Update(actionKey(tea.KeyRight))
		more = next.(Model)
	}
	next, cmd := more.Update(actionKey(tea.KeyEnter))
	detail := next.(Model)
	if cmd != nil || !strings.Contains(detail.ViewString(), m.Snapshot.Status.Items[0].Path) || strings.Contains(strings.ToLower(detail.Status), "copied") {
		t.Fatalf("error detail missing or falsely copied:\n%s", detail.ViewString())
	}
	retryMore, _ := keyboardAction(t, m, actionIndex(t, m, "More"))
	retryIdx := actionIndex(t, retryMore, "Retry")
	for retryMore.ActionIndex != retryIdx {
		next, _ := retryMore.Update(actionKey(tea.KeyRight))
		retryMore = next.(Model)
	}
	next, preview := retryMore.Update(actionKey(tea.KeyEnter))
	retry := next.(Model)
	if preview == nil || service.syncCalls != 0 || !retry.Busy {
		t.Fatalf("Retry skipped dry-run preview cmd=%v calls=%d", preview != nil, service.syncCalls)
	}
	next, _ = retry.Update(preview())
	retry = next.(Model)
	if service.syncCalls != 1 || !service.lastSync.DryRun || retry.Mode != ModeConfirm {
		t.Fatalf("Retry preview request=%+v mode=%s", service.lastSync, retry.Mode)
	}
	next, applySync := retry.Update(actionKey(tea.KeyEnter))
	retry = next.(Model)
	if applySync == nil || service.syncCalls != 1 || !retry.MutationBusy {
		t.Fatal("Retry confirm was not deferred")
	}
	_ = applySync()
	if service.syncCalls != 2 || service.lastSync.DryRun {
		t.Fatalf("Retry apply request=%+v calls=%d", service.lastSync, service.syncCalls)
	}

	refreshModel, refresh := mouseAction(t, m, actionIndex(t, m, "Refresh"))
	if refresh == nil || service.snapshotCalls != 0 || !refreshModel.Busy {
		t.Fatalf("Refresh not deferred: cmd=%v calls=%d", refresh != nil, service.snapshotCalls)
	}
	_ = refresh()
	if service.snapshotCalls != 1 || !service.lastSnapshot.ForceRefresh || service.lastSnapshot.Offline {
		t.Fatalf("Refresh request=%+v calls=%d", service.lastSnapshot, service.snapshotCalls)
	}
}
