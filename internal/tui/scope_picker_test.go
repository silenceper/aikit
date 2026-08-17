package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
)

func chooseRowByName(t *testing.T, m Model, name string, mouse bool) (Model, tea.Cmd) {
	t.Helper()
	index := -1
	for i, current := range m.rows() {
		if current.Name == name {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatalf("choice %q missing from rows: %+v", name, m.rows())
	}
	m.Cursor = index
	if mouse {
		regions := m.hitRegions()
		next, cmd := m.Update(click(regions.Rows[index].X, regions.Rows[index].Y))
		return next.(Model), cmd
	}
	next, cmd := m.Update(actionKey(tea.KeyEnter))
	return next.(Model), cmd
}

func chooseVisibleAction(t *testing.T, m Model, label string, mouse bool) (Model, tea.Cmd) {
	t.Helper()
	index := actionIndex(t, m, label)
	if mouse {
		return mouseAction(t, m, index)
	}
	if m.Focus != FocusActions {
		return keyboardAction(t, m, index)
	}
	for m.ActionIndex != index {
		next, _ := m.Update(actionKey(tea.KeyRight))
		m = next.(Model)
	}
	next, cmd := m.Update(actionKey(tea.KeyEnter))
	return next.(Model), cmd
}

func TestStructuredScopePickerLibraryBatchKeyboardMouse(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		t.Run(map[bool]string{false: "keyboard", true: "mouse"}[mouse], func(t *testing.T) {
			service := &fakeService{}
			m := NewModel(nil, service, &fakeMigration{}, ViewLibrary, ActionNone)
			m.Snapshot, m.Width, m.Height = testSnapshot(), 110, 30
			m.Selected["library:acme/alpha"] = true
			m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
			var cmd tea.Cmd
			m, cmd = chooseVisibleAction(t, m, "Enable selected", mouse)
			if cmd != nil || m.Mode != ModeScopePicker {
				t.Fatalf("scope picker mode=%s cmd=%v prompt=%q", m.Mode, cmd != nil, m.Input.Prompt)
			}
			view := m.ViewString()
			for _, want := range []string{"All agents", "Global / codex", "Project / aikit / Common", "Project / aikit / codex"} {
				if !strings.Contains(view, want) {
					t.Fatalf("scope picker missing %q:\n%s", want, view)
				}
			}
			for _, forbidden := range []string{"agent:name", "project:name", "project-agent:"} {
				if strings.Contains(view, forbidden) {
					t.Fatalf("scope picker exposed %q:\n%s", forbidden, view)
				}
			}
			m, cmd = chooseRowByName(t, m, "Global / codex", mouse)
			if cmd == nil || service.previewBatchCalls != 0 {
				t.Fatalf("batch preview cmd=%v calls=%d", cmd != nil, service.previewBatchCalls)
			}
			next, _ := m.Update(cmd())
			m = next.(Model)
			want := app.BatchRequest{Operation: app.BatchEnable, Bindings: []app.BindingRequest{{SkillID: "acme/alpha", Agent: "codex"}}}
			if !reflect.DeepEqual(service.lastBatchPreview, want) || m.Mode != ModeConfirm {
				t.Fatalf("batch preview request=%+v want=%+v mode=%s", service.lastBatchPreview, want, m.Mode)
			}
		})
	}
}

func TestStructuredScopePickerPresetAndProjectApply(t *testing.T) {
	t.Run("presets page", func(t *testing.T) {
		service := &fakeService{}
		m := NewModel(nil, service, &fakeMigration{}, ViewPresets, ActionNone)
		m.Snapshot, m.Width, m.Height = testSnapshot(), 110, 30
		m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
		m, _ = chooseVisibleAction(t, m, "Apply", false)
		if m.Mode != ModeScopePicker {
			t.Fatalf("preset apply mode=%s prompt=%q", m.Mode, m.Input.Prompt)
		}
		m, cmd := chooseRowByName(t, m, "Project / aikit / Common", false)
		if cmd == nil {
			t.Fatal("preset apply preview was not deferred")
		}
		_ = cmd()
		want := app.PresetMutationRequest{Operation: app.PresetApply, Name: "review", Binding: app.BindingRequest{Preset: "review", Project: "aikit"}}
		if !reflect.DeepEqual(service.lastPresetMutationPreview, want) {
			t.Fatalf("preset apply request=%+v want=%+v", service.lastPresetMutationPreview, want)
		}
	})

	t.Run("project workspace", func(t *testing.T) {
		service := &fakeService{}
		m := projectWorkspaceModel(service)
		m.Scope = Scope{Project: "aikit", Level: "project-targets"}
		m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
		m, _ = chooseVisibleAction(t, m, "Apply preset", false)
		if m.Mode != ModePresetPicker {
			t.Fatalf("project preset picker mode=%s", m.Mode)
		}
		m, cmd := chooseRowByName(t, m, "review", false)
		if cmd != nil || m.Mode != ModeScopePicker {
			t.Fatalf("project target picker mode=%s cmd=%v", m.Mode, cmd != nil)
		}
		if len(m.rows()) != 2 {
			t.Fatalf("project target choices=%+v", m.rows())
		}
		m, cmd = chooseRowByName(t, m, "Project / aikit / codex", false)
		if cmd == nil {
			t.Fatal("project preset preview was not deferred")
		}
		_ = cmd()
		want := app.PresetMutationRequest{Operation: app.PresetApply, Name: "review", Binding: app.BindingRequest{Preset: "review", Project: "aikit", Agent: "codex"}}
		if !reflect.DeepEqual(service.lastPresetMutationPreview, want) {
			t.Fatalf("project preset request=%+v want=%+v", service.lastPresetMutationPreview, want)
		}
	})
}

func TestStructuredScopePickerCancelAndGlobalWorkspace(t *testing.T) {
	service := &fakeService{}
	m := NewModel(nil, service, &fakeMigration{}, ViewPresets, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 110, 30
	m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
	m, _ = chooseVisibleAction(t, m, "Apply", false)
	next, cmd := m.Update(actionKey(tea.KeyEsc))
	m = next.(Model)
	if cmd != nil || service.previewBatchCalls+service.previewPresetMutationCalls+service.mutatePresetCalls+service.batchCalls != 0 || m.Mode == ModeScopePicker {
		t.Fatalf("picker cancel mutated: cmd=%v previewBatch=%d presetPreview=%d preset=%d batch=%d mode=%s", cmd != nil, service.previewBatchCalls, service.previewPresetMutationCalls, service.mutatePresetCalls, service.batchCalls, m.Mode)
	}

	global := NewModel(nil, service, &fakeMigration{}, ViewWorkspaces, ActionNone)
	global.Snapshot, global.Width, global.Height = testSnapshot(), 110, 30
	next, cmd = global.Update(actionKey(tea.KeyEnter))
	global = next.(Model)
	if cmd != nil || global.Scope.Level != "workspace-global" || global.Detail || len(global.rows()) != len(global.Snapshot.Config.Library.Skills) {
		t.Fatalf("global workspace scope=%+v detail=%v rows=%d cmd=%v", global.Scope, global.Detail, len(global.rows()), cmd != nil)
	}
}
