package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
)

func TestNavigationShowsDirectNumberShortcuts(t *testing.T) {
	m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
	m.Width, m.Height = 100, 24

	view := stripANSI(m.ViewString())
	for _, want := range []string{"1 Overview", "2 Library", "3 Workspaces", "4 Presets", "5 Migration", "6 Status"} {
		if !strings.Contains(view, want) {
			t.Fatalf("navigation hides shortcut %q:\n%s", want, view)
		}
	}
}

func TestProjectTargetPresetUsesSelectedExactScope(t *testing.T) {
	for _, tt := range []struct {
		name   string
		cursor int
		want   app.BindingRequest
	}{
		{name: "common", cursor: 0, want: app.BindingRequest{Preset: "review", Project: "aikit"}},
		{name: "agent", cursor: 1, want: app.BindingRequest{Preset: "review", Project: "aikit", Agent: "codex"}},
	} {
		for _, mouse := range []bool{false, true} {
			t.Run(tt.name+map[bool]string{false: "-keyboard", true: "-mouse"}[mouse], func(t *testing.T) {
				service := &fakeService{snapshot: testSnapshot()}
				m := loadedModel(t, service, &fakeMigration{})
				m.switchView(ViewWorkspaces)
				m.Scope = Scope{Level: "project-targets", Project: "aikit"}
				m.Cursor, m.Width, m.Height = tt.cursor, 38, 18
				if !strings.Contains(strings.Join(m.primaryActions(), ","), "Apply preset") {
					t.Fatalf("project target actions = %v", m.primaryActions())
				}
				var cmd tea.Cmd
				if mouse {
					m, cmd = mouseActionByLabel(t, m, "Apply preset")
				} else {
					m, cmd = chooseVisibleAction(t, m, "Apply preset", false)
				}
				if cmd != nil || m.Mode != ModePresetPicker {
					t.Fatalf("preset picker mode=%s cmd=%v", m.Mode, cmd != nil)
				}
				m, cmd = chooseRowByName(t, m, "review", mouse)
				if cmd == nil || m.Mode == ModeScopePicker {
					t.Fatalf("selected exact target was requested again: mode=%s cmd=%v", m.Mode, cmd != nil)
				}
				_ = cmd()
				if !reflect.DeepEqual(service.lastPresetMutationPreview.Binding, tt.want) {
					t.Fatalf("preset binding=%+v want=%+v", service.lastPresetMutationPreview.Binding, tt.want)
				}
			})
		}
	}
}

func TestFocusedEmptyStateActionIsNamedInFooter(t *testing.T) {
	m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
	m.switchView(ViewWorkspaces)
	m.Snapshot.Config.Projects = nil
	m.Scope = Scope{Level: "workspace-projects"}
	m.Focus, m.ActionIndex = FocusActions, 0

	if got := m.footer(); !strings.Contains(got, "Enter Create project") {
		t.Fatalf("focused empty-state footer = %q", got)
	}
}

func TestWorkspaceAgentSummaryDescribesSupportedAgents(t *testing.T) {
	m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
	m.switchView(ViewWorkspaces)
	rows := m.rows()
	if len(rows) < 2 || rows[1].Name != "Agents" || rows[1].State != "5 supported" {
		t.Fatalf("workspace agent summary = %+v", rows)
	}
}

func TestWorkspaceAgentExposesPresetWithoutMore(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		t.Run(map[bool]string{false: "keyboard", true: "mouse"}[mouse], func(t *testing.T) {
			m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
			m.switchView(ViewWorkspaces)
			m.Scope = Scope{Level: "workspace-agents"}
			m.Cursor, m.Width, m.Height = 0, 38, 18
			wantAgent := m.rows()[m.Cursor].ID
			if got := strings.Join(m.primaryActions(), ","); !strings.Contains(got, "Apply preset") {
				t.Fatalf("agent primary actions = %s", got)
			}
			var next Model
			var cmd tea.Cmd
			if mouse {
				next, cmd = mouseActionByLabel(t, m, "Apply preset")
			} else {
				next, cmd = chooseVisibleAction(t, m, "Apply preset", false)
			}
			if cmd != nil {
				t.Fatal("agent preset picker unexpectedly returned command")
			}
			if next.Mode != ModePresetPicker || next.Picker.Agent != wantAgent {
				t.Fatalf("agent preset picker = mode %s picker %+v", next.Mode, next.Picker)
			}
		})
	}

	// Keep a real KeyMsg in this audit so the shortcut assertions cannot be
	// satisfied by render-only labels disconnected from input routing.
	m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if next.(Model).ActiveView != ViewWorkspaces {
		t.Fatal("rendered 3 shortcut does not open Workspaces")
	}
}

func TestWorkspaceWithoutPresetsOffersCreationInsteadOfEmptyPicker(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		t.Run(map[bool]string{false: "keyboard", true: "mouse"}[mouse], func(t *testing.T) {
			snapshot := testSnapshot()
			snapshot.Config.Presets = nil
			m := loadedModel(t, &fakeService{snapshot: snapshot}, &fakeMigration{})
			m.switchView(ViewWorkspaces)
			m.Scope = Scope{Level: "workspace-agents"}
			m.Cursor, m.Width, m.Height = 0, 38, 18
			if strings.Contains(strings.Join(m.primaryActions(), ","), "Apply preset") {
				t.Fatalf("empty preset picker remains advertised: %v", m.primaryActions())
			}
			var cmd tea.Cmd
			if mouse {
				m, cmd = mouseActionByLabel(t, m, "Create preset")
			} else {
				m, cmd = chooseVisibleAction(t, m, "Create preset", false)
			}
			if cmd != nil || m.Mode != ModeInput || m.Input.Kind != inputPresetCreate {
				t.Fatalf("create preset route mode=%s kind=%s cmd=%v", m.Mode, m.Input.Kind, cmd != nil)
			}
		})
	}
}
