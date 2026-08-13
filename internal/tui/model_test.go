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
	snapshot           app.Snapshot
	snapshotCalls      int
	updateCalls        int
	lastUpdate         app.UpdateRequest
	removeCalls        int
	lastRemove         app.RemoveRequest
	projectRemoveCalls int
	lastProjectRemove  app.ProjectRemoveRequest
	presetCalls        int
	lastPreset         app.PresetRequest
}

func (f *fakeService) Remove(_ context.Context, request app.RemoveRequest) (app.Result, error) {
	f.removeCalls++
	f.lastRemove = request
	return app.Result{Changed: true}, nil
}

func (f *fakeService) RemoveProject(_ context.Context, request app.ProjectRemoveRequest) (app.Result, error) {
	f.projectRemoveCalls++
	f.lastProjectRemove = request
	return app.Result{Changed: true}, nil
}

func (f *fakeService) PutPreset(_ context.Context, request app.PresetRequest) (app.Result, error) {
	f.presetCalls++
	f.lastPreset = request
	return app.Result{Changed: true}, nil
}

func (f *fakeService) Snapshot(context.Context, app.StatusRequest) (app.Snapshot, error) {
	f.snapshotCalls++
	return f.snapshot, nil
}

func (f *fakeService) Update(_ context.Context, request app.UpdateRequest) (app.Result, error) {
	f.updateCalls++
	f.lastUpdate = request
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
	return updated.(Model)
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

	for input, want := range map[string]View{"1": ViewLibrary, "2": ViewAgents, "3": ViewProjects, "4": ViewPresets, "5": ViewStatus} {
		m, _ = apply(m, input)
		if m.ActiveView != want {
			t.Fatalf("key %s selected %s, want %s", input, m.ActiveView, want)
		}
	}
	m, _ = apply(m, "1")
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

	m, _ = apply(m, "2")
	for i, row := range m.rows() {
		if row.ID == "codex" {
			m.Cursor = i
			break
		}
	}
	m, _ = apply(m, "enter")
	if m.Scope.Agent != "codex" || len(m.rows()) != 2 {
		t.Fatalf("agent subview = %#v rows=%d", m.Scope, len(m.rows()))
	}
	m, _ = apply(m, "esc")
	m, _ = apply(m, "3")
	m, _ = apply(m, "enter")
	if m.Scope.Project != "aikit" || m.rows()[0].ID != "common" {
		t.Fatalf("project subview = %#v rows=%#v", m.Scope, m.rows())
	}
	m, _ = apply(m, "esc")
	m, _ = apply(m, "4")
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
	if !strings.Contains(m.Status, "updated") || refresh == nil {
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
	m, _ = apply(m, "5")
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

func TestBusyRejectsKeysExceptCtrlC(t *testing.T) {
	base := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
	base.Busy = true
	for _, input := range []string{"enter", "space", "u", "S", "s", "1", "/", "esc"} {
		next, cmd := apply(base, input)
		if cmd != nil || next.ActiveView != base.ActiveView || next.Mode != base.Mode || next.Cursor != base.Cursor {
			t.Fatalf("busy model accepted %q: next=%+v cmd=%v", input, next, cmd != nil)
		}
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
	m, _ = apply(m, "d")
	if m.Mode != ModeConfirm || service.removeCalls != 0 {
		t.Fatalf("delete mutated before confirm: mode=%s calls=%d", m.Mode, service.removeCalls)
	}
	m, _ = apply(m, "esc")
	if service.removeCalls != 0 {
		t.Fatal("cancelled delete mutated")
	}
	m, _ = apply(m, "d")
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
	m, _ = apply(m, "3")
	m, _ = apply(m, "x")
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

func TestPresetMemberSpaceTogglesViaAppCommand(t *testing.T) {
	service := &fakeService{snapshot: testSnapshot()}
	m := loadedModel(t, service, &fakeMigration{})
	m, _ = apply(m, "4")
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
	if cmd == nil || service.presetCalls != 0 {
		t.Fatal("preset toggle was not deferred")
	}
	_ = cmd()
	if service.presetCalls != 1 || service.lastPreset.Name != "review" || len(service.lastPreset.Skills) != 1 || service.lastPreset.Skills[0] != "acme/beta" || service.lastPreset.Remove {
		t.Fatalf("preset add request = %#v", service.lastPreset)
	}
}
