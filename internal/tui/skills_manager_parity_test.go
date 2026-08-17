package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSkillsManagerParityRoutesAreVisibleAndStructured(t *testing.T) {
	t.Run("project creation asks only for a directory", func(t *testing.T) {
		m, cmd := invokeProjectAction(t, projectWorkspaceModel(&fakeService{}), "Create project", false)
		if cmd != nil || m.Mode != ModeInput {
			t.Fatalf("create entry mode=%s cmd=%v", m.Mode, cmd != nil)
		}
		if got := m.Input.Prompt; got != "Project directory" {
			t.Fatalf("create prompt=%q, want path-only Project directory", got)
		}
	})

	t.Run("preset apply uses a typed target picker", func(t *testing.T) {
		m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewPresets, ActionNone)
		m.Snapshot, m.Width, m.Height = testSnapshot(), 110, 30
		m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
		m.ActionIndex, m.Focus = actionIndex(t, m, "Apply"), FocusActions
		next, cmd := m.Update(actionKey(tea.KeyEnter))
		m = next.(Model)
		if cmd != nil {
			t.Fatalf("apply target selection unexpectedly started backend command")
		}
		if m.Mode == ModeInput || strings.Contains(m.Input.Prompt, "agent:name") || strings.Contains(m.Input.Prompt, "project-agent:") {
			t.Fatalf("preset apply exposed encoded target input mode=%s prompt=%q", m.Mode, m.Input.Prompt)
		}
	})

	t.Run("library batch uses a typed target picker", func(t *testing.T) {
		m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
		m.Snapshot, m.Width, m.Height = testSnapshot(), 110, 30
		m.Selected["library:acme/alpha"] = true
		m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
		m.ActionIndex, m.Focus = actionIndex(t, m, "Enable selected"), FocusActions
		next, cmd := m.Update(actionKey(tea.KeyEnter))
		m = next.(Model)
		if cmd != nil {
			t.Fatalf("batch target selection unexpectedly started backend command")
		}
		if m.Mode == ModeInput || strings.Contains(m.Input.Prompt, "agent:name") || strings.Contains(m.Input.Prompt, "project-agent:") {
			t.Fatalf("batch enable exposed encoded target input mode=%s prompt=%q", m.Mode, m.Input.Prompt)
		}
	})

	t.Run("global workspace is an operable skill collection", func(t *testing.T) {
		m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewWorkspaces, ActionNone)
		m.Snapshot, m.Width, m.Height = testSnapshot(), 110, 30
		rows := m.rows()
		for index := range rows {
			if rows[index].ID == "global" {
				m.Cursor = index
				break
			}
		}
		next, cmd := m.perform(uiActivate)
		m = next.(Model)
		if cmd != nil || m.Scope.Level != "workspace-global" || m.Detail {
			t.Fatalf("global activation scope=%+v detail=%v cmd=%v", m.Scope, m.Detail, cmd != nil)
		}
		if len(m.rows()) != len(m.Snapshot.Config.Library.Skills) {
			t.Fatalf("global workspace rows=%d, want %d library skills", len(m.rows()), len(m.Snapshot.Config.Library.Skills))
		}
	})
}
