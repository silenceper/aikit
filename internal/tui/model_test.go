package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/status"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

type fakeService struct {
	app.Service
	snapshot                   app.Snapshot
	snapshotCalls              int
	lastSnapshot               app.StatusRequest
	updateCalls                int
	lastUpdate                 app.UpdateRequest
	updateResult               app.Result
	removeCalls                int
	lastRemove                 app.RemoveRequest
	projectRemoveCalls         int
	lastProjectRemove          app.ProjectRemoveRequest
	projectRemoveResult        app.Result
	projectRemovePreviewCalls  int
	lastProjectRemovePreview   app.ProjectRemoveRequest
	projectRemovePreview       app.MutationPreview
	projectPreviewCalls        int
	lastProjectPreview         app.ProjectEditRequest
	projectPreview             app.ProjectEditPreview
	projectRegistrationCalls   int
	lastProjectRegistration    app.ProjectRegistrationRequest
	projectRegistration        app.ProjectRegistrationPreview
	projectRegistrationFunc    func(app.ProjectRegistrationRequest) app.ProjectRegistrationPreview
	projectEditCalls           int
	lastProjectEdit            app.ProjectEditRequest
	presetCalls                int
	lastPreset                 app.PresetRequest
	previewAddCalls            int
	lastPreviewAdd             app.AddPreviewRequest
	addPreview                 app.AddPreview
	addPreviewFunc             func(app.AddPreviewRequest) app.AddPreview
	addCalls                   int
	lastAdd                    app.AddRequest
	detailCalls                int
	detail                     app.SkillDetail
	previewBindingCalls        int
	lastBindingPreview         app.BindingPreviewRequest
	bindingPreview             app.MutationPreview
	bindingCalls               int
	lastBinding                app.BindingRequest
	lastBindingEnable          bool
	previewRemoveCalls         int
	removePreview              app.MutationPreview
	removePreviews             map[string]app.MutationPreview
	previewPresetCalls         int
	lastPresetPreview          app.PresetPreviewRequest
	presetPreview              app.MutationPreview
	previewPresetMutationCalls int
	lastPresetMutationPreview  app.PresetMutationRequest
	presetMutationPreview      app.MutationPreview
	mutatePresetCalls          int
	lastPresetMutation         app.PresetMutationRequest
	syncCalls                  int
	lastSync                   app.SyncRequest
	syncResult                 app.Result
	configurationCalls         int
	configuration              app.ConfigurationDetail
	validateConfigurationCalls int
	configurationValidation    app.ConfigurationValidation
	compareCalls               int
	lastCompare                app.CompareRequest
	compareResult              app.CompareResult
	batchCalls                 int
	lastBatch                  app.BatchRequest
	batchResult                app.BatchResult
	previewBatchCalls          int
	lastBatchPreview           app.BatchRequest
	batchPreview               app.BatchPreview
	previewRecoveryCalls       int
	lastRecoveryPreview        app.RecoveryRequest
	recoveryPreview            app.RecoveryPreview
	resumeRecoveryCalls        int
	lastRecoveryResume         app.RecoveryRequest
	recoveryResult             app.RecoveryResult
}

func (f *fakeService) PreviewRecovery(_ context.Context, request app.RecoveryRequest) (app.RecoveryPreview, error) {
	f.previewRecoveryCalls++
	f.lastRecoveryPreview = request
	return f.recoveryPreview, nil
}

func (f *fakeService) ResumeRecovery(_ context.Context, request app.RecoveryRequest) (app.RecoveryResult, error) {
	f.resumeRecoveryCalls++
	f.lastRecoveryResume = request
	return f.recoveryResult, nil
}

func (f *fakeService) Batch(_ context.Context, request app.BatchRequest) (app.BatchResult, error) {
	f.batchCalls++
	f.lastBatch = request
	if f.batchResult.Items != nil || f.batchResult.Issues != nil {
		return f.batchResult, nil
	}
	return app.BatchResult{Result: app.Result{Changed: true}}, nil
}

func (f *fakeService) PreviewBatch(_ context.Context, request app.BatchRequest) (app.BatchPreview, error) {
	f.previewBatchCalls++
	f.lastBatchPreview = request
	if f.batchPreview.Title != "" || f.batchPreview.Summary != "" || len(f.batchPreview.Items) > 0 {
		return f.batchPreview, nil
	}
	return app.BatchPreview{MutationPreview: app.MutationPreview{Title: "Batch preview", Summary: "Exact batch", RequiresConfirmation: true}}, nil
}

func (f *fakeService) PreviewAdd(_ context.Context, request app.AddPreviewRequest) (app.AddPreview, error) {
	f.previewAddCalls++
	f.lastPreviewAdd = request
	if f.addPreviewFunc != nil {
		return f.addPreviewFunc(request), nil
	}
	return f.addPreview, nil
}

func (f *fakeService) Add(_ context.Context, request app.AddRequest) (app.Result, error) {
	f.addCalls++
	f.lastAdd = request
	return app.Result{Changed: true}, nil
}

func (f *fakeService) SkillDetail(_ context.Context, _ string) (app.SkillDetail, error) {
	f.detailCalls++
	return f.detail, nil
}

func (f *fakeService) PreviewBinding(_ context.Context, request app.BindingPreviewRequest) (app.MutationPreview, error) {
	f.previewBindingCalls++
	f.lastBindingPreview = request
	return f.bindingPreview, nil
}

func (f *fakeService) Enable(_ context.Context, request app.BindingRequest) (app.Result, error) {
	f.bindingCalls++
	f.lastBinding = request
	f.lastBindingEnable = true
	return app.Result{Changed: true}, nil
}

func (f *fakeService) Disable(_ context.Context, request app.BindingRequest) (app.Result, error) {
	f.bindingCalls++
	f.lastBinding = request
	f.lastBindingEnable = false
	return app.Result{Changed: true}, nil
}

func (f *fakeService) PreviewRemove(_ context.Context, request app.RemoveRequest) (app.MutationPreview, error) {
	f.previewRemoveCalls++
	if preview, ok := f.removePreviews[request.SkillID]; ok {
		return preview, nil
	}
	return f.removePreview, nil
}

func (f *fakeService) PreviewPreset(_ context.Context, request app.PresetPreviewRequest) (app.MutationPreview, error) {
	f.previewPresetCalls++
	f.lastPresetPreview = request
	return f.presetPreview, nil
}

func (f *fakeService) PreviewPresetMutation(_ context.Context, request app.PresetMutationRequest) (app.MutationPreview, error) {
	f.previewPresetMutationCalls++
	f.lastPresetMutationPreview = request
	if f.presetMutationPreview.Title != "" || f.presetMutationPreview.Summary != "" || len(f.presetMutationPreview.References) > 0 || len(f.presetMutationPreview.AffectedScopes) > 0 || len(f.presetMutationPreview.Plan.Actions) > 0 || f.presetMutationPreview.RequiresForce {
		return f.presetMutationPreview, nil
	}
	return f.presetPreview, nil
}

func (f *fakeService) MutatePreset(_ context.Context, request app.PresetMutationRequest) (app.Result, error) {
	f.mutatePresetCalls++
	f.lastPresetMutation = request
	return app.Result{Changed: true}, nil
}

func (f *fakeService) Sync(_ context.Context, request app.SyncRequest) (app.Result, error) {
	f.syncCalls++
	f.lastSync = request
	return f.syncResult, nil
}

func (f *fakeService) Configuration(_ context.Context) (app.ConfigurationDetail, error) {
	f.configurationCalls++
	return f.configuration, nil
}

func (f *fakeService) ValidateConfiguration(_ context.Context) (app.ConfigurationValidation, error) {
	f.validateConfigurationCalls++
	return f.configurationValidation, nil
}

func (f *fakeService) Compare(_ context.Context, request app.CompareRequest) (app.CompareResult, error) {
	f.compareCalls++
	f.lastCompare = request
	return f.compareResult, nil
}

func (f *fakeService) Remove(_ context.Context, request app.RemoveRequest) (app.Result, error) {
	f.removeCalls++
	f.lastRemove = request
	return app.Result{Changed: true}, nil
}

func (f *fakeService) RemoveProject(_ context.Context, request app.ProjectRemoveRequest) (app.Result, error) {
	f.projectRemoveCalls++
	f.lastProjectRemove = request
	if f.projectRemoveResult.Changed || len(f.projectRemoveResult.Warnings) > 0 || len(f.projectRemoveResult.Link.Warnings) > 0 || len(f.projectRemoveResult.Link.Issues) > 0 {
		return f.projectRemoveResult, nil
	}
	return app.Result{Changed: true}, nil
}

func (f *fakeService) PreviewProjectRemove(_ context.Context, request app.ProjectRemoveRequest) (app.MutationPreview, error) {
	f.projectRemovePreviewCalls++
	f.lastProjectRemovePreview = request
	return f.projectRemovePreview, nil
}

func (f *fakeService) PreviewProjectEdit(_ context.Context, request app.ProjectEditRequest) (app.ProjectEditPreview, error) {
	f.projectPreviewCalls++
	f.lastProjectPreview = request
	return f.projectPreview, nil
}

func (f *fakeService) PreviewProjectRegistration(_ context.Context, request app.ProjectRegistrationRequest) (app.ProjectRegistrationPreview, error) {
	f.projectRegistrationCalls++
	f.lastProjectRegistration = request
	if f.projectRegistrationFunc != nil {
		return f.projectRegistrationFunc(request), nil
	}
	return f.projectRegistration, nil
}

func (f *fakeService) EditProject(_ context.Context, request app.ProjectEditRequest) (app.Result, error) {
	f.projectEditCalls++
	f.lastProjectEdit = request
	return app.Result{Changed: true}, nil
}

func (f *fakeService) PutPreset(_ context.Context, request app.PresetRequest) (app.Result, error) {
	f.presetCalls++
	f.lastPreset = request
	return app.Result{Changed: true}, nil
}

func (f *fakeService) Snapshot(_ context.Context, request app.StatusRequest) (app.Snapshot, error) {
	f.snapshotCalls++
	f.lastSnapshot = request
	return f.snapshot, nil
}

func (f *fakeService) Update(_ context.Context, request app.UpdateRequest) (app.Result, error) {
	f.updateCalls++
	f.lastUpdate = request
	if f.updateResult.Updates.Results != nil || f.updateResult.Updates.Warnings != nil {
		return f.updateResult, nil
	}
	return app.Result{Changed: true}, nil
}

type fakeMigration struct {
	app.MigrationService
	scanCalls int
	requests  []app.ScanRequest
	result    app.ScanResult
}

func (f *fakeMigration) Scan(_ context.Context, request app.ScanRequest) (app.ScanResult, error) {
	f.scanCalls++
	f.requests = append(f.requests, request)
	return f.result, nil
}

func (f *fakeMigration) Inventory(_ context.Context, request app.InventoryRequest) <-chan app.InventoryEvent {
	events := make(chan app.InventoryEvent, 1)
	events <- app.InventoryEvent{Generation: request.Generation, Done: true}
	close(events)
	return events
}

func testSnapshot() app.Snapshot {
	return app.Snapshot{
		Config: config.Config{
			Library: config.Library{Skills: []config.Skill{
				{ID: "acme/alpha", Name: "alpha", Source: "https://example.test/acme.git", Resolved: strings.Repeat("a", 40)},
				{ID: "acme/beta", Name: "beta", Source: "https://example.test/acme.git", Resolved: strings.Repeat("b", 40)},
			}},
			Agents:   map[string]config.Binding{"codex": {Skills: []string{"acme/alpha"}}},
			Projects: []config.Project{{Name: "aikit", Path: "/work/aikit", Agents: []string{"codex"}, Binding: config.Binding{Skills: []string{"acme/beta"}}}},
			Presets:  []config.Preset{{Name: "review", Skills: []string{"acme/alpha"}}},
		},
		Status: status.Report{Items: []status.Item{{
			Kind: status.Unmanaged, Name: "loose", Path: "/work/.codex/skills/loose", Scope: config.Scope{Agent: "codex", Project: "aikit"},
		}}},
		Updates: updatecheck.CheckReport{Results: []updatecheck.Result{{SkillID: "acme/alpha", Current: strings.Repeat("a", 40), Remote: strings.Repeat("c", 40), State: updatecheck.StateUpdateAvailable}}},
	}
}

func loadedModel(t *testing.T, service *fakeService, migration *fakeMigration) Model {
	t.Helper()
	m := NewModel(context.Background(), service, migration, ViewLibrary, ActionNone)
	cmd := m.Init()
	if service.snapshotCalls != 0 {
		t.Fatal("Init blocked on Snapshot; backend call must be deferred to tea.Cmd")
	}
	updated, _ := m.Update(cmd())
	result := updated.(Model)
	result.cancelInventory()
	result.Inventory.Loading = false
	result.Busy, result.MutationBusy = false, false
	result.Activity = Activity{}
	return result
}

func key(value string) tea.KeyMsg {
	switch value {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+q":
		return tea.KeyMsg{Type: tea.KeyCtrlQ}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
	}
}

func apply(m Model, value string) (Model, tea.Cmd) {
	next, cmd := m.Update(key(value))
	return next.(Model), cmd
}

func TestModelViewKeysNavigationFilterHelpAndDetail(t *testing.T) {
	service := &fakeService{snapshot: testSnapshot()}
	m := loadedModel(t, service, &fakeMigration{})

	for input, want := range map[string]View{"1": ViewOverview, "2": ViewLibrary, "3": ViewPresets, "4": ViewStatus} {
		m, _ = apply(m, input)
		if m.ActiveView != want {
			t.Fatalf("key %s selected %s, want %s", input, m.ActiveView, want)
		}
	}
	m, _ = apply(m, "2")
	m, _ = apply(m, "j")
	if m.Cursor != 1 {
		t.Fatalf("j cursor = %d, want 1", m.Cursor)
	}
	m, _ = apply(m, "up")
	if m.Cursor != 0 {
		t.Fatalf("up cursor = %d, want 0", m.Cursor)
	}
	m, _ = apply(m, "enter")
	if !m.Detail {
		t.Fatal("enter did not open library detail")
	}
	m, _ = apply(m, "esc")
	if m.Detail {
		t.Fatal("escape did not close detail")
	}
	m, _ = apply(m, "/")
	m, _ = apply(m, "b")
	m, _ = apply(m, "e")
	m, _ = apply(m, "enter")
	if got := m.rows(); len(got) != 1 || got[0].ID != "acme/beta" {
		t.Fatalf("filtered rows = %#v", got)
	}
	m, _ = apply(m, "?")
	if !m.Help {
		t.Fatal("? did not open help")
	}
	m, _ = apply(m, "esc")
	if m.Help {
		t.Fatal("escape did not close help")
	}
}

func TestAgentsProjectsAndPresetsEnterSubViews(t *testing.T) {
	m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})

	m.switchView(ViewWorkspaces)
	m.Scope.Level = "workspace-agents"
	for i, row := range m.rows() {
		if row.ID == "codex" {
			m.Cursor = i
			break
		}
	}
	m, _ = apply(m, "enter")
	if m.Scope.Agent != "codex" || len(m.rows()) != 2 {
		t.Fatalf("agent subview = %#v cursor=%d active=%q rows=%#v", m.Scope, m.Cursor, m.rows()[m.Cursor].ID, m.rows())
	}
	m, _ = apply(m, "esc")
	m.switchView(ViewWorkspaces)
	m.Scope.Level = "workspace-projects"
	m, _ = apply(m, "enter")
	foundCommon := false
	for _, row := range m.rows() {
		foundCommon = foundCommon || row.ID == "common"
	}
	if m.Scope.Project != "aikit" || !foundCommon {
		t.Fatalf("project subview = %#v rows=%#v", m.Scope, m.rows())
	}
	m, _ = apply(m, "esc")
	m, _ = apply(m, "3")
	m, _ = apply(m, "enter")
	if m.Scope.Preset != "review" || len(m.rows()) != 2 {
		t.Fatalf("preset subview = %#v rows=%#v", m.Scope, m.rows())
	}
}

func TestUpdateCancelAndAsyncConfirmation(t *testing.T) {
	service := &fakeService{snapshot: testSnapshot()}
	m := loadedModel(t, service, &fakeMigration{})
	m, _ = apply(m, "u")
	if m.Mode != ModeUpdates || len(m.rows()) != 1 {
		t.Fatalf("update mode=%s rows=%d", m.Mode, len(m.rows()))
	}
	m, _ = apply(m, "space")
	m, _ = apply(m, "enter")
	if m.Mode != ModeConfirm || service.updateCalls != 0 {
		t.Fatalf("confirmation mutated early: mode=%s calls=%d", m.Mode, service.updateCalls)
	}
	m, _ = apply(m, "esc")
	if service.updateCalls != 0 || m.Mode != ModeUpdates {
		t.Fatalf("cancel mutated: mode=%s calls=%d", m.Mode, service.updateCalls)
	}
	m, _ = apply(m, "enter")
	var cmd tea.Cmd
	m, cmd = apply(m, "enter")
	if cmd == nil || service.updateCalls != 0 {
		t.Fatalf("confirm must return async command without blocking: cmd=%v calls=%d", cmd != nil, service.updateCalls)
	}
	msg := cmd()
	if service.updateCalls != 1 || !service.lastUpdate.Confirmed || len(service.lastUpdate.SkillIDs) != 1 {
		t.Fatalf("update request = %#v calls=%d", service.lastUpdate, service.updateCalls)
	}
	next, refresh := m.Update(msg)
	m = next.(Model)
	if !strings.Contains(strings.ToLower(m.Status), "updated") || refresh == nil {
		t.Fatalf("async result status=%q refresh=%v", m.Status, refresh != nil)
	}
}

func TestScanAdoptPreviewCancelAndConfirm(t *testing.T) {
	first := "/work/.cursor/skills/loose"
	second := "/work/.codex/skills/loose"
	migration := &fakeMigration{result: app.ScanResult{Items: []app.ScanItem{
		{Origin: "g/cursor", Target: first, Skill: config.Skill{ID: "local/loose", Name: "loose"}},
		{Origin: "g/codex", Target: second, Skill: config.Skill{ID: "local/loose", Name: "loose"}},
	}}}
	m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, migration)
	m, cmd := apply(m, "S")
	if cmd == nil || migration.scanCalls != 0 {
		t.Fatalf("scan must be async: cmd=%v calls=%d", cmd != nil, migration.scanCalls)
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.Mode != ModeScan || migration.scanCalls != 1 || len(m.rows()) != 2 || !migration.requests[0].DryRun {
		t.Fatalf("scan result mode=%s calls=%d rows=%d", m.Mode, migration.scanCalls, len(m.rows()))
	}
	m, _ = apply(m, "space")
	m.Cursor = 1
	if m.Selected[m.rows()[1].selectionKey()] {
		t.Fatal("duplicate skill ids shared one selection key")
	}
	m.Cursor = 0
	m, _ = apply(m, "enter")
	if m.Mode != ModeConfirm || migration.scanCalls != 1 {
		t.Fatalf("adopt confirmation mutated early: mode=%s calls=%d", m.Mode, migration.scanCalls)
	}
	m, _ = apply(m, "esc")
	if migration.scanCalls != 1 || m.Mode != ModeScan {
		t.Fatalf("adopt cancel mutated: mode=%s calls=%d", m.Mode, migration.scanCalls)
	}
	m, _ = apply(m, "enter")
	_, adopt := apply(m, "enter")
	if adopt == nil || migration.scanCalls != 1 {
		t.Fatal("adopt confirmation did not produce deferred command")
	}
	_ = adopt()
	if migration.scanCalls != 2 || !migration.requests[1].Adopt || len(migration.requests[1].Skills) != 0 || len(migration.requests[1].Targets) != 1 || migration.requests[1].Targets[0] != first {
		t.Fatalf("adopt request = %#v", migration.requests)
	}
}

func TestStatusAdoptPreviewsOnlySelectedTarget(t *testing.T) {
	target := "/work/.codex/skills/loose"
	migration := &fakeMigration{result: app.ScanResult{Items: []app.ScanItem{{Origin: "p/aikit/codex", Target: target, Skill: config.Skill{ID: "local/loose", Name: "loose"}}}}}
	m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, migration)
	m, _ = apply(m, "4")
	m, cmd := apply(m, "A")
	if cmd == nil || migration.scanCalls != 0 {
		t.Fatal("status adopt preview must be deferred")
	}
	_ = cmd()
	request := migration.requests[0]
	if !request.DryRun || request.Agent != "codex" || request.Project != "aikit" || len(request.Targets) != 1 || request.Targets[0] != target {
		t.Fatalf("status adopt preview request = %#v", request)
	}
}

func TestBusyRejectsSubmissionsButAllowsBrowsing(t *testing.T) {
	base := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
	base.Busy = true
	base.Activity = Activity{Kind: ActivityReading, Label: "Loading"}
	for _, input := range []string{"enter", "space", "u", "S", "s", "r"} {
		next, cmd := apply(base, input)
		if cmd != nil || next.ActiveView != base.ActiveView || next.Mode != base.Mode || next.Cursor != base.Cursor {
			t.Fatalf("busy model accepted %q: next=%+v cmd=%v", input, next, cmd != nil)
		}
	}
	next, cmd := apply(base, "1")
	if cmd != nil || next.ActiveView != ViewOverview {
		t.Fatalf("busy model blocked browsing: view=%s cmd=%v", next.ActiveView, cmd != nil)
	}
	_, quit := apply(base, "ctrl+c")
	if quit == nil {
		t.Fatal("busy model rejected ctrl-c")
	}
}

func TestFilterRestoresParentSubtableMode(t *testing.T) {
	m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
	m, _ = apply(m, "u")
	m, _ = apply(m, "/")
	m, _ = apply(m, "a")
	if rows := m.rows(); len(rows) != 1 || rows[0].ID != "acme/alpha" {
		t.Fatalf("update filter switched tables: %+v", rows)
	}
	m, _ = apply(m, "enter")
	if m.Mode != ModeUpdates {
		t.Fatalf("filter restored mode %s, want %s", m.Mode, ModeUpdates)
	}

	m.Mode = ModeScan
	m.Scan = app.ScanResult{Items: []app.ScanItem{{Origin: "g/codex", Target: "/tmp/loose", Skill: config.Skill{ID: "local/loose", Name: "loose"}}}}
	m.Filter = ""
	m, _ = apply(m, "/")
	m, _ = apply(m, "l")
	if rows := m.rows(); len(rows) != 1 || rows[0].ID != "local/loose" {
		t.Fatalf("scan filter switched tables: %+v", rows)
	}
	m, _ = apply(m, "esc")
	if m.Mode != ModeScan {
		t.Fatalf("filter restored mode %s, want %s", m.Mode, ModeScan)
	}
}

func TestLibraryDeleteAndProjectRemoveRequireConfirmation(t *testing.T) {
	service := &fakeService{snapshot: testSnapshot()}
	m := loadedModel(t, service, &fakeMigration{})
	m, preview := apply(m, "d")
	if preview == nil {
		t.Fatal("delete preview was not deferred")
	}
	next, _ := m.Update(preview())
	m = next.(Model)
	if m.Mode != ModeConfirm || service.removeCalls != 0 {
		t.Fatalf("delete mutated before confirm: mode=%s calls=%d", m.Mode, service.removeCalls)
	}
	m, _ = apply(m, "esc")
	if service.removeCalls != 0 {
		t.Fatal("cancelled delete mutated")
	}
	m, preview = apply(m, "d")
	next, _ = m.Update(preview())
	m = next.(Model)
	m, cmd := apply(m, "enter")
	if cmd == nil || service.removeCalls != 0 {
		t.Fatal("confirmed delete was not deferred")
	}
	_ = cmd()
	if service.removeCalls != 1 || service.lastRemove.SkillID != "acme/alpha" || service.lastRemove.Force {
		t.Fatalf("delete request = %#v", service.lastRemove)
	}

	service.snapshotCalls = 0
	m = loadedModel(t, service, &fakeMigration{})
	m.switchView(ViewWorkspaces)
	m.Scope.Level = "workspace-projects"
	m, preview = apply(m, "x")
	if preview == nil || service.projectRemovePreviewCalls != 0 {
		t.Fatal("project remove preview was not deferred")
	}
	next, _ = m.Update(preview())
	m = next.(Model)
	if m.Mode != ModeConfirm || service.projectRemoveCalls != 0 {
		t.Fatal("project remove mutated before confirm")
	}
	m, cmd = apply(m, "enter")
	if cmd == nil || service.projectRemoveCalls != 0 {
		t.Fatal("project remove was not deferred")
	}
	_ = cmd()
	if service.projectRemoveCalls != 1 || service.lastProjectRemove.Project != "aikit" || !service.lastProjectRemove.Confirmed {
		t.Fatalf("project remove request = %#v", service.lastProjectRemove)
	}
}

func TestPresetMemberSpaceStagesWithoutMutation(t *testing.T) {
	service := &fakeService{snapshot: testSnapshot()}
	m := loadedModel(t, service, &fakeMigration{})
	m, _ = apply(m, "3")
	m, _ = apply(m, "enter")
	rows := m.rows()
	if len(rows) != 2 {
		t.Fatalf("preset member table must show all library skills: %+v", rows)
	}
	for i, row := range rows {
		if row.ID == "acme/beta" {
			m.Cursor = i
		}
	}
	m, cmd := apply(m, "space")
	if cmd != nil || service.presetCalls != 0 || service.mutatePresetCalls != 0 || !m.Selected["acme/beta"] {
		t.Fatalf("preset toggle was not staged: cmd=%v putCalls=%d mutationCalls=%d selected=%v", cmd != nil, service.presetCalls, service.mutatePresetCalls, m.Selected)
	}
}
