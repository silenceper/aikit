package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/pkg/config"
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
		m.Snapshot, m.Scope, m.Width, m.Height = testSnapshot(), Scope{Level: "workspace-global"}, 110, 30
		if len(m.rows()) != len(m.Snapshot.Config.Library.Skills) {
			t.Fatalf("global workspace rows=%d, want %d library skills", len(m.rows()), len(m.Snapshot.Config.Library.Skills))
		}
	})

	t.Run("global agent exposes skills batch and preset", func(t *testing.T) {
		m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewWorkspaces, ActionNone)
		m.Snapshot, m.Scope = testSnapshot(), Scope{Agent: "codex", Level: "agent-skills"}
		for _, action := range []string{"Select skills", "More"} {
			if !strings.Contains(strings.Join(m.primaryActions(), " "), action) {
				t.Fatalf("global agent route missing %q: %v", action, m.primaryActions())
			}
		}
		m.enterMore()
		if !strings.Contains(strings.Join(m.primaryActions(), " "), "Apply preset") {
			t.Fatalf("global agent preset route missing: %v", m.primaryActions())
		}
	})

	t.Run("project common agent management and preset routes", func(t *testing.T) {
		m := projectWorkspaceModel(&fakeService{})
		m.Scope = Scope{Project: "aikit", Level: "project-targets"}
		for _, action := range []string{"Open", "Apply preset", "More"} {
			if !strings.Contains(strings.Join(m.primaryActions(), " "), action) {
				t.Fatalf("project route missing %q: %v", action, m.primaryActions())
			}
		}
		m.enterMore()
		if !strings.Contains(strings.Join(m.primaryActions(), " "), "Manage agents") {
			t.Fatalf("project agent management route missing: %v", m.primaryActions())
		}
		m.Mode, m.Focus = ModeTable, FocusList
		for index, current := range m.rows() {
			if current.ID == "common" {
				m.Cursor = index
				break
			}
		}
		next, _ := m.perform(uiActivate)
		m = next.(Model)
		if m.Scope.Level != "project-skills" || m.Scope.Agent != "" || !strings.Contains(strings.Join(m.primaryActions(), " "), "Select skills") {
			t.Fatalf("project common route scope=%+v actions=%v", m.Scope, m.primaryActions())
		}
		m.Scope = Scope{Project: "aikit", Level: "project-targets"}
		for index, current := range m.rows() {
			if current.ID == "codex" {
				m.Cursor = index
				break
			}
		}
		next, _ = m.perform(uiActivate)
		m = next.(Model)
		if m.Scope.Level != "project-skills" || m.Scope.Agent != "codex" || !strings.Contains(strings.Join(m.primaryActions(), " "), "Select skills") {
			t.Fatalf("project agent route scope=%+v actions=%v", m.Scope, m.primaryActions())
		}
	})

	t.Run("preset creation migration adopt and recovery are discoverable", func(t *testing.T) {
		presets := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewPresets, ActionNone)
		presets.Snapshot = testSnapshot()
		presets.enterMore()
		if !strings.Contains(strings.Join(presets.primaryActions(), " "), "Create preset") {
			t.Fatalf("preset create route missing from More: %v", presets.primaryActions())
		}

		migration := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewMigration, ActionNone)
		migration.Inventory.Items = []app.ScanItem{{Key: "origin", Target: "/work/skill", Action: app.ScanActionAdopt, Skill: config.Skill{ID: "local/skill", Name: "skill"}}}
		if !strings.Contains(strings.Join(migration.primaryActions(), " "), "Adopt") {
			t.Fatalf("migration adopt route missing: %v", migration.primaryActions())
		}

		service := &fakeService{}
		recovery := NewModel(nil, service, &fakeMigration{}, ViewOverview, ActionNone)
		snapshot := testSnapshot()
		snapshot.Config.PendingOperations = []config.PendingOperation{{ID: "recover-1", Kind: config.OperationCleanup}}
		next, cmd := recovery.Update(snapshotMsg{snapshot: snapshot})
		recovery = next.(Model)
		if cmd == nil || !recovery.Busy || recovery.pendingRecovery.OperationIDs[0] != "recover-1" {
			t.Fatalf("recovery route cmd=%v busy=%v request=%+v", cmd != nil, recovery.Busy, recovery.pendingRecovery)
		}
	})

	t.Run("all visible prompts avoid encoded mini languages", func(t *testing.T) {
		models := []Model{
			NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone),
			NewModel(nil, &fakeService{}, &fakeMigration{}, ViewWorkspaces, ActionNone),
			NewModel(nil, &fakeService{}, &fakeMigration{}, ViewPresets, ActionNone),
		}
		for index := range models {
			models[index].Snapshot, models[index].Width, models[index].Height = testSnapshot(), 110, 30
			for _, forbidden := range []string{"pipe-separated", "agent:name", "project:name", "project-agent:"} {
				if strings.Contains(models[index].ViewString()+models[index].Input.Prompt, forbidden) {
					t.Fatalf("model %d exposes encoded prompt %q", index, forbidden)
				}
			}
		}
	})
}
