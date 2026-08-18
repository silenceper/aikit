package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/status"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

func TestOverviewQuickActionsKeyboardAndMouseShareExistingFlows(t *testing.T) {
	base := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	base.Snapshot, base.Width, base.Height = testSnapshot(), 120, 24

	keyboard := base
	keyboard.switchOverviewSection(overviewQuick)
	keyboard.Focus, keyboard.ActionIndex = FocusCollectionActions, 0
	next, keyboardCmd := keyboard.Update(tea.KeyMsg{Type: tea.KeyEnter})
	keyboard = next.(Model)

	regions := base.hitRegions()
	next, mouseCmd := base.Update(click(regions.OverviewQuick.Buttons[0].X, regions.OverviewQuick.Buttons[0].Y))
	mouse := next.(Model)
	if keyboardCmd != nil || mouseCmd != nil || keyboard.Mode != ModeInput || mouse.Mode != ModeInput || keyboard.Input.Kind != inputAddSource || mouse.Input.Kind != inputAddSource {
		t.Fatalf("add skill parity keyboard=%+v mouse=%+v", keyboard.Input, mouse.Input)
	}
}

func TestOverviewCheckUpdatesStaysOnDashboardAndShowsTypedResults(t *testing.T) {
	service := &fakeService{updateResult: app.Result{Updates: updatecheck.CheckReport{Results: []updatecheck.Result{{SkillID: "acme/alpha", Current: strings.Repeat("a", 40), Remote: strings.Repeat("c", 40), State: updatecheck.StateUpdateAvailable}}}}}
	m := NewModel(context.Background(), service, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot = testSnapshot()
	m.Snapshot.Updates = updatecheck.CheckReport{}
	m.OverviewSection, m.Focus, m.ActionIndex = overviewUpdates, FocusCollectionActions, 0

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || !m.Busy {
		t.Fatalf("check did not start: busy=%v cmd=%v", m.Busy, cmd != nil)
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if service.updateCalls != 1 || !service.lastUpdate.CheckOnly || !service.lastUpdate.Refresh || m.ActiveView != ViewOverview || m.Mode != ModeTable || m.OverviewSection != overviewUpdates {
		t.Fatalf("check result request=%+v view=%s mode=%s section=%s", service.lastUpdate, m.ActiveView, m.Mode, m.OverviewSection)
	}
	if len(m.overviewDashboard().Updates) != 1 {
		t.Fatalf("typed updates=%+v", m.overviewDashboard().Updates)
	}
}

func TestOverviewUpdateBatchUsesCompleteTokensAndSinglePreview(t *testing.T) {
	service := &fakeService{}
	m := NewModel(context.Background(), service, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot = testSnapshot()
	m.Snapshot.Config.Library.Skills[0].Ref = &config.Ref{Kind: "branch", Value: "main"}
	m.OverviewSection = overviewUpdates
	m.Selected["overview:update:acme/alpha"] = true
	m.Focus, m.ActionIndex = FocusCollectionActions, 1

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || !m.Busy || service.previewBatchCalls != 0 {
		t.Fatalf("preview was not deferred: busy=%v cmd=%v calls=%d", m.Busy, cmd != nil, service.previewBatchCalls)
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	wantExpected := app.ExpectedUpdate{Ref: &config.Ref{Kind: "branch", Value: "main"}, Resolved: strings.Repeat("a", 40), Remote: strings.Repeat("c", 40)}
	if service.previewBatchCalls != 1 || service.lastBatchPreview.Operation != app.BatchUpdate || !reflect.DeepEqual(service.lastBatchPreview.Expected["acme/alpha"], wantExpected) || m.Mode != ModeConfirm {
		t.Fatalf("batch preview=%+v mode=%s", service.lastBatchPreview, m.Mode)
	}

	next, cancelCmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cancelCmd != nil || service.batchCalls != 0 || m.ActiveView != ViewOverview {
		t.Fatalf("cancel mutated: cmd=%v batch=%d view=%s", cancelCmd != nil, service.batchCalls, m.ActiveView)
	}
}

func TestOverviewLocalSelectionBuildsExactMigrationPreview(t *testing.T) {
	migration := &fakeMigration{result: app.ScanResult{Items: []app.ScanItem{{Key: "safe", Origin: "g/codex", Target: "/work/safe", Action: app.ScanActionImport}}}}
	m := NewModel(context.Background(), &fakeService{}, migration, ViewOverview, ActionNone)
	m.Width, m.Height, m.OverviewSection = 120, 24, overviewLocal
	m.Inventory.Items = []app.ScanItem{
		{Key: "safe", Origin: "g/codex", Target: "/work/safe", ContentHash: "hash", ObjectID: "object", RootObjectID: "root", State: app.ScanStateUnmanaged, Action: app.ScanActionImport, Skill: config.Skill{ID: "local/safe", Name: "safe"}},
		{Key: "conflict", Origin: "g/claude", Target: "/work/conflict", State: app.ScanStateNameConflict, Action: app.ScanActionConflict, Skill: config.Skill{ID: "local/conflict", Name: "conflict"}},
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(Model)
	if !m.Selected["overview:local:safe"] {
		t.Fatalf("safe row not selected: %v", m.Selected)
	}
	m.Focus, m.ActionIndex = FocusCollectionActions, 1
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || !m.Busy || migration.scanCalls != 0 {
		t.Fatalf("local preview was not deferred: busy=%v cmd=%v calls=%d", m.Busy, cmd != nil, migration.scanCalls)
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	request := migration.requests[0]
	if migration.scanCalls != 1 || !request.DryRun || request.Adopt || len(request.Selectors) != 1 || request.Selectors[0].Key != "safe" || request.Selectors[0].ExpectedObjectID != "object" || request.Targets[0] != "/work/safe" || m.Mode != ModeConfirm {
		t.Fatalf("exact migration request=%+v mode=%s", request, m.Mode)
	}
	next, cancelCmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cancelCmd != nil || migration.scanCalls != 1 || m.ActiveView != ViewOverview || m.OverviewSection != overviewLocal {
		t.Fatalf("cancel changed migration calls=%d view=%s section=%s", migration.scanCalls, m.ActiveView, m.OverviewSection)
	}
}

func TestOverviewLocalUnsafeRowsCannotJoinExecutableSelection(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.OverviewSection = overviewLocal
	m.Inventory.Items = []app.ScanItem{{Key: "conflict", Origin: "g/codex", Target: "/work/conflict", State: app.ScanStateNameConflict, Action: app.ScanActionConflict, Skill: config.Skill{ID: "local/conflict", Name: "conflict"}}}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(Model)
	if cmd != nil || len(m.Selected) != 0 || !strings.Contains(m.Status, "review only") {
		t.Fatalf("unsafe selection changed state: selected=%v status=%q", m.Selected, m.Status)
	}
}

func TestOverviewHealthOpensExactRetainedStatusRow(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot, m.OverviewSection = testSnapshot(), overviewHealth
	tasks := m.overviewDashboard().Health
	index := -1
	for i, task := range tasks {
		if task.DestinationView == ViewStatus {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("status task missing")
	}
	m.Cursor = index
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil || m.ActiveView != ViewStatus || m.activeKey() != tasks[index].DestinationKey {
		t.Fatalf("health route view=%s key=%q want=%q", m.ActiveView, m.activeKey(), tasks[index].DestinationKey)
	}
}

func TestOverviewHealthActionAdoptsExactUnmanagedTarget(t *testing.T) {
	migration := &fakeMigration{}
	m := NewModel(context.Background(), &fakeService{}, migration, ViewOverview, ActionNone)
	m.Snapshot, m.OverviewSection = testSnapshot(), overviewHealth
	m.Focus, m.ActionIndex = FocusCollectionActions, 0
	if got := m.overviewSectionActions(overviewHealth); len(got) == 0 || got[0] != "Adopt" {
		t.Fatalf("health actions=%v want Adopt first", got)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || !m.Busy || migration.scanCalls != 0 {
		t.Fatalf("adopt preview not deferred: busy=%v cmd=%v calls=%d", m.Busy, cmd != nil, migration.scanCalls)
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	request := migration.requests[0]
	if migration.scanCalls != 1 || !request.DryRun || !request.Adopt || request.Project != "aikit" || request.Agent != "codex" || !reflect.DeepEqual(request.Targets, []string{"/work/.codex/skills/loose"}) || m.Mode != ModeConfirm {
		t.Fatalf("adopt request=%+v mode=%s", request, m.Mode)
	}
	next, cancelCmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cancelCmd != nil || migration.scanCalls != 1 || m.ActiveView != ViewOverview || m.OverviewSection != overviewHealth {
		t.Fatalf("cancel changed calls=%d view=%s section=%s", migration.scanCalls, m.ActiveView, m.OverviewSection)
	}
}

func TestOverviewHealthActionBuildsSyncAndRecoveryPreviews(t *testing.T) {
	t.Run("sync", func(t *testing.T) {
		service := &fakeService{}
		m := NewModel(context.Background(), service, &fakeMigration{}, ViewOverview, ActionNone)
		m.Snapshot, m.OverviewSection = testSnapshot(), overviewHealth
		m.Snapshot.Status.Items = []status.Item{{Kind: status.Missing, Name: "missing", SkillID: "acme/alpha", Path: "/work/.codex/skills/alpha", Scope: config.Scope{Agent: "codex"}}}
		m.Focus, m.ActionIndex = FocusCollectionActions, 0
		if got := m.overviewSectionActions(overviewHealth); len(got) == 0 || got[0] != "Sync preview" {
			t.Fatalf("health actions=%v want Sync preview first", got)
		}
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(Model)
		if cmd == nil || !m.Busy || service.syncCalls != 0 {
			t.Fatalf("sync preview not deferred: busy=%v cmd=%v calls=%d", m.Busy, cmd != nil, service.syncCalls)
		}
	})

	t.Run("recovery", func(t *testing.T) {
		service := &fakeService{}
		m := NewModel(context.Background(), service, &fakeMigration{}, ViewOverview, ActionNone)
		m.Snapshot, m.OverviewSection = testSnapshot(), overviewHealth
		m.Snapshot.Config.PendingOperations = []config.PendingOperation{{ID: "recover-1"}}
		m.Focus, m.ActionIndex = FocusCollectionActions, 0
		if got := m.overviewSectionActions(overviewHealth); len(got) == 0 || got[0] != "Review recovery" {
			t.Fatalf("health actions=%v want Review recovery first", got)
		}
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(Model)
		if cmd == nil || !m.Busy || service.previewRecoveryCalls != 0 {
			t.Fatalf("recovery preview not deferred: busy=%v cmd=%v calls=%d", m.Busy, cmd != nil, service.previewRecoveryCalls)
		}
		next, _ = m.Update(cmd())
		m = next.(Model)
		if service.previewRecoveryCalls != 1 || !reflect.DeepEqual(service.lastRecoveryPreview.OperationIDs, []string{"recover-1"}) || m.Mode != ModeConfirm {
			t.Fatalf("recovery request=%+v mode=%s", service.lastRecoveryPreview, m.Mode)
		}
	})
}
