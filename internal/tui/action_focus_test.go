package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/status"
	"github.com/silenceper/aikit/pkg/config"
)

func actionKey(key tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: key} }

func focusActionsWithTab(t *testing.T, m Model) Model {
	t.Helper()
	for step := 0; step < 2; step++ {
		next, _ := m.Update(actionKey(tea.KeyTab))
		m = next.(Model)
		if m.Focus == FocusActions {
			if m.ActionIndex != 0 {
				t.Fatalf("Tab action=%d, want 0", m.ActionIndex)
			}
			return m
		}
		if m.Focus == FocusDetail && !m.Detail && ComputeLayout(m.Width, m.Height).Detail.Empty() {
			t.Fatalf("Tab entered invisible detail at width=%d", m.Width)
		}
	}
	t.Fatalf("Tab did not reach actions; focus=%s", m.Focus)
	return m
}

func keyboardAction(t *testing.T, m Model, index int) (Model, tea.Cmd) {
	t.Helper()
	m = focusActionsWithTab(t, m)
	for i := 0; i < index; i++ {
		next, _ := m.Update(actionKey(tea.KeyRight))
		m = next.(Model)
	}
	if m.ActionIndex != index {
		t.Fatalf("right navigation selected %d, want %d", m.ActionIndex, index)
	}
	next, cmd := m.Update(actionKey(tea.KeyEnter))
	return next.(Model), cmd
}

func mouseAction(t *testing.T, m Model, index int) (Model, tea.Cmd) {
	t.Helper()
	regions := m.hitRegions()
	if index >= len(regions.Actions) {
		t.Fatalf("action %d has no rendered hit region; actions=%v regions=%d", index, m.primaryActions(), len(regions.Actions))
	}
	next, cmd := m.Update(click(regions.Actions[index].X, regions.Actions[index].Y))
	got := next.(Model)
	return got, cmd
}

func TestActionFocusTabLeftRightEnterAndEsc(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 100, 20
	m = focusActionsWithTab(t, m)

	next, _ := m.Update(actionKey(tea.KeyRight))
	m = next.(Model)
	if m.ActionIndex != 1 || !strings.Contains(stripANSI(m.ViewString()), "{Add source}") {
		t.Fatalf("right action=%d or focus not rendered:\n%s", m.ActionIndex, m.ViewString())
	}
	next, _ = m.Update(actionKey(tea.KeyLeft))
	m = next.(Model)
	if m.ActionIndex != 0 {
		t.Fatalf("left action=%d, want 0", m.ActionIndex)
	}
	next, _ = m.Update(actionKey(tea.KeyTab))
	m = next.(Model)
	if m.Focus != FocusActions || m.ActionIndex != 1 {
		t.Fatalf("Tab within actions focus=%s action=%d", m.Focus, m.ActionIndex)
	}
	next, _ = m.Update(actionKey(tea.KeyEsc))
	m = next.(Model)
	if m.Focus != FocusList {
		t.Fatalf("Esc focus=%s, want list", m.Focus)
	}
}

func TestConfigurationOverlayRendersActualKeyboardFocus(t *testing.T) {
	service := &fakeService{}
	m := configurationModel(service)
	m.Width, m.Height = 80, 16
	next, cmd := m.Update(actionKey(tea.KeyRight))
	m = next.(Model)
	if cmd != nil || m.Focus != FocusActions || m.ActionIndex != 1 || !strings.Contains(stripANSI(m.ViewString()), "{Reload}") {
		t.Fatalf("configuration focus=%s action=%d cmd=%v:\n%s", m.Focus, m.ActionIndex, cmd != nil, m.ViewString())
	}
	next, cmd = m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if cmd == nil || !m.Busy {
		t.Fatalf("Enter did not invoke visually focused Reload: cmd=%v busy=%v", cmd != nil, m.Busy)
	}
}

func TestTabNeverEntersInvisibleDetailPane(t *testing.T) {
	for _, width := range []int{80, 120} {
		m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
		m.Snapshot, m.Width, m.Height = testSnapshot(), width, 20
		detailVisible := !ComputeLayout(width, m.Height).Detail.Empty()
		next, _ := m.Update(actionKey(tea.KeyTab))
		m = next.(Model)
		if detailVisible {
			if m.Focus != FocusDetail {
				t.Fatalf("width=%d Tab focus=%s, want detail", width, m.Focus)
			}
			next, _ = m.Update(actionKey(tea.KeyTab))
			m = next.(Model)
		}
		if m.Focus != FocusActions || m.ActionIndex != 0 || !strings.Contains(stripANSI(m.ViewString()), "{Open}") {
			t.Fatalf("width=%d Tab focus=%s action=%d:\n%s", width, m.Focus, m.ActionIndex, m.ViewString())
		}
		if strings.Contains(stripANSI(m.ViewString()), "> [ ] alpha") {
			t.Fatalf("width=%d list retained strong focus while actions focused:\n%s", width, m.ViewString())
		}
		next, _ = m.Update(actionKey(tea.KeyShiftTab))
		m = next.(Model)
		wantFocus := FocusList
		if detailVisible {
			wantFocus = FocusDetail
		}
		if m.Focus != wantFocus {
			t.Fatalf("width=%d Shift-Tab focus=%s, want %s", width, m.Focus, wantFocus)
		}
	}
}

func TestRenderedActionEndToEndKeyboardMouseParity(t *testing.T) {
	newBase := func(view View) (Model, *fakeService) {
		service := &fakeService{snapshot: testSnapshot(), compareResult: app.CompareResult{Left: app.CompareSide{Skill: config.Skill{ID: "acme/alpha"}}, Right: app.CompareSide{Skill: config.Skill{ID: "acme/beta"}}}}
		m := NewModel(nil, service, &fakeMigration{}, view, ActionNone)
		m.Snapshot, m.Width, m.Height = testSnapshot(), 100, 20
		return m, service
	}

	tests := []struct {
		name   string
		setup  func(Model) Model
		label  string
		assert func(*testing.T, Model, tea.Cmd, *fakeService)
	}{
		{
			name: "compare", label: "Compare",
			setup: func(m Model) Model {
				m.switchView(ViewMigration)
				m.Inventory.Items = []app.ScanItem{{Key: "conflict", MatchedLibraryID: "acme/alpha", Skill: config.Skill{ID: "acme/beta", Name: "beta"}, State: app.ScanStateNameConflict, Action: app.ScanActionConflict}}
				return m
			},
			assert: func(t *testing.T, _ Model, cmd tea.Cmd, service *fakeService) {
				if cmd == nil {
					t.Fatal("compare returned no command")
				}
				_ = cmd()
				if service.lastCompare != (app.CompareRequest{LeftSkillID: "acme/alpha", RightSkillID: "acme/beta"}) {
					t.Fatalf("compare request=%+v", service.lastCompare)
				}
			},
		},
		{
			name: "ignore", label: "Ignore",
			setup: func(m Model) Model {
				m.switchView(ViewMigration)
				m.Inventory.Items = testInventoryItems()
				return m
			},
			assert: func(t *testing.T, m Model, cmd tea.Cmd, _ *fakeService) {
				if cmd != nil || !m.Ignored["one"] {
					t.Fatalf("ignore cmd=%v ignored=%v", cmd != nil, m.Ignored)
				}
			},
		},
		{name: "add", label: "Add source", setup: func(m Model) Model { return m }, assert: func(t *testing.T, m Model, cmd tea.Cmd, _ *fakeService) {
			if cmd != nil || m.Mode != ModeInput || m.Input.Kind != inputAddSource {
				t.Fatalf("add mode=%s input=%s cmd=%v", m.Mode, m.Input.Kind, cmd != nil)
			}
		}},
		{name: "more", label: "More", setup: func(m Model) Model { return m }, assert: func(t *testing.T, m Model, cmd tea.Cmd, _ *fakeService) {
			if cmd != nil || m.Mode != ModeMore {
				t.Fatalf("more mode=%s cmd=%v", m.Mode, cmd != nil)
			}
		}},
		{name: "sync", label: "Sync preview", setup: func(m Model) Model {
			m.switchView(ViewStatus)
			m.Snapshot.Status.Items[0].Kind = status.Missing
			return m
		}, assert: func(t *testing.T, _ Model, cmd tea.Cmd, service *fakeService) {
			if cmd == nil {
				t.Fatal("sync returned no command")
			}
			_ = cmd()
			if service.lastSync != (app.SyncRequest{DryRun: true}) {
				t.Fatalf("sync request=%+v", service.lastSync)
			}
		}},
		{name: "create", label: "Create preset", setup: func(m Model) Model { m.switchView(ViewPresets); m.Snapshot.Config.Presets = nil; return m }, assert: func(t *testing.T, m Model, cmd tea.Cmd, _ *fakeService) {
			if cmd != nil || m.Mode != ModeInput || m.Input.Kind != inputPresetCreate {
				t.Fatalf("create mode=%s input=%s cmd=%v", m.Mode, m.Input.Kind, cmd != nil)
			}
		}},
		{name: "save", label: "Save", setup: func(m Model) Model {
			m.switchView(ViewPresets)
			m.Scope = Scope{Preset: "review", Level: "preset-skills"}
			m.Selected["acme/alpha"] = true
			return m
		}, assert: func(t *testing.T, m Model, cmd tea.Cmd, _ *fakeService) {
			if cmd == nil || !m.Busy || m.pendingPreset.Operation != app.PresetEditMembers {
				t.Fatalf("save mode=%s preset=%+v cmd=%v", m.Mode, m.pendingPreset, cmd != nil)
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyboardBase, keyboardService := newBase(ViewLibrary)
			keyboardBase = tt.setup(keyboardBase)
			index := actionIndex(t, keyboardBase, tt.label)
			keyboard, keyboardCmd := keyboardAction(t, keyboardBase, index)
			tt.assert(t, keyboard, keyboardCmd, keyboardService)

			mouseBase, mouseService := newBase(ViewLibrary)
			mouseBase = tt.setup(mouseBase)
			mouse, mouseCmd := mouseAction(t, mouseBase, index)
			tt.assert(t, mouse, mouseCmd, mouseService)
			if keyboard.Mode != mouse.Mode || keyboard.Focus != mouse.Focus || keyboard.ActionIndex != mouse.ActionIndex {
				t.Fatalf("keyboard/mouse state mismatch keyboard=(%s,%s,%d) mouse=(%s,%s,%d)", keyboard.Mode, keyboard.Focus, keyboard.ActionIndex, mouse.Mode, mouse.Focus, mouse.ActionIndex)
			}
		})
	}
}

func actionIndex(t *testing.T, m Model, label string) int {
	t.Helper()
	for i, action := range m.primaryActions() {
		if action == label {
			return i
		}
	}
	t.Fatalf("rendered action %q missing from %v", label, m.primaryActions())
	return -1
}
