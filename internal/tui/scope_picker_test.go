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
	pickerMode := m.Mode == ModeScopePicker || m.Mode == ModePresetPicker
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
		m = next.(Model)
		if !pickerMode || cmd != nil {
			return m, cmd
		}
		return chooseVisibleAction(t, m, "Apply", true)
	}
	next, cmd := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if !pickerMode || cmd != nil {
		return m, cmd
	}
	return chooseVisibleAction(t, m, "Apply", false)
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

func pickerRowIndex(t *testing.T, m Model, name string) int {
	t.Helper()
	for index, current := range m.rows() {
		if current.Name == name {
			return index
		}
	}
	t.Fatalf("picker row %q missing from %+v", name, m.rows())
	return -1
}

func TestPickerChoiceDoesNotApplyUntilExplicitAction(t *testing.T) {
	service := &fakeService{}
	m := NewModel(nil, service, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 110, 30
	m.Selected["library:acme/alpha"] = true
	m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
	m, _ = chooseVisibleAction(t, m, "Enable selected", false)
	if m.Mode != ModeScopePicker {
		t.Fatalf("mode=%s, want scope picker", m.Mode)
	}

	index := pickerRowIndex(t, m, "Global / codex")
	regions := m.hitRegions()
	next, cmd := m.Update(click(regions.Rows[index].X+1, regions.Rows[index].Y))
	m = next.(Model)
	if cmd != nil || m.Cursor != index || m.Picker.Selected != index || m.Focus != FocusList {
		t.Fatalf("mouse choice cursor=%d selected=%d focus=%s cmd=%v", m.Cursor, m.Picker.Selected, m.Focus, cmd != nil)
	}
	if service.previewBatchCalls != 0 || service.batchCalls != 0 {
		t.Fatalf("mouse choice submitted preview=%d mutation=%d", service.previewBatchCalls, service.batchCalls)
	}

	m.Cursor = pickerRowIndex(t, m, "Project / aikit / Common")
	next, cmd = m.Update(actionKey(tea.KeySpace))
	m = next.(Model)
	if cmd != nil || m.Picker.Selected != m.Cursor || m.Focus != FocusList {
		t.Fatalf("space choice selected=%d cursor=%d focus=%s cmd=%v", m.Picker.Selected, m.Cursor, m.Focus, cmd != nil)
	}

	m.Cursor = pickerRowIndex(t, m, "Project / aikit / codex")
	next, cmd = m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if cmd != nil || m.Picker.Selected != m.Cursor || m.Focus != FocusActions {
		t.Fatalf("enter choice selected=%d cursor=%d focus=%s cmd=%v", m.Picker.Selected, m.Cursor, m.Focus, cmd != nil)
	}
	if got := m.primaryActions(); len(got) == 0 || got[0] != "Apply" {
		t.Fatalf("picker actions=%v, want Apply first", got)
	}

	next, cmd = m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if cmd == nil || service.previewBatchCalls != 0 {
		t.Fatalf("apply cmd=%v preview calls before execution=%d", cmd != nil, service.previewBatchCalls)
	}
	_ = cmd()
	if service.previewBatchCalls != 1 {
		t.Fatalf("apply preview calls=%d, want 1", service.previewBatchCalls)
	}
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

func TestGlobalWorkspacePresetPickerOnlyOffersGlobalTargets(t *testing.T) {
	service := &fakeService{}
	m := NewModel(nil, service, &fakeMigration{}, ViewWorkspaces, ActionNone)
	m.Snapshot, m.Scope, m.Width, m.Height = testSnapshot(), Scope{Level: "workspace-global"}, 110, 30
	m, _ = chooseVisibleAction(t, m, "Apply preset", false)
	if m.Mode != ModePresetPicker {
		t.Fatalf("preset picker mode=%s", m.Mode)
	}
	m, cmd := chooseRowByName(t, m, "review", false)
	if cmd != nil || m.Mode != ModeScopePicker {
		t.Fatalf("global target picker mode=%s cmd=%v", m.Mode, cmd != nil)
	}
	for _, current := range m.rows() {
		if strings.HasPrefix(current.Name, "Project /") {
			t.Fatalf("global workspace offered project target %q", current.Name)
		}
	}
	m, preview := chooseRowByName(t, m, "All agents", false)
	if preview == nil {
		t.Fatal("all-agents preset preview was not deferred")
	}
	_ = preview()
	if service.lastBatchPreview.Operation != app.BatchEnable || len(service.lastBatchPreview.Bindings) != 5 {
		t.Fatalf("all-agent preset batch=%+v", service.lastBatchPreview)
	}
	for _, binding := range service.lastBatchPreview.Bindings {
		if binding.Preset != "review" || binding.Project != "" || binding.Agent == "" {
			t.Fatalf("invalid global preset binding=%+v", binding)
		}
	}
}
