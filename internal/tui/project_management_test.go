package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/pkg/config"
)

func openProjectMoreAction(t *testing.T, m Model, label string, mouse bool) (Model, tea.Cmd) {
	t.Helper()
	more := actionIndex(t, m, "More")
	if mouse {
		m, _ = mouseAction(t, m, more)
	} else {
		m, _ = keyboardAction(t, m, more)
	}
	index := actionIndex(t, m, label)
	if mouse {
		return mouseAction(t, m, index)
	}
	for m.ActionIndex != index {
		next, _ := m.Update(actionKey(tea.KeyRight))
		m = next.(Model)
	}
	next, cmd := m.Update(actionKey(tea.KeyEnter))
	return next.(Model), cmd
}

func TestProjectRenameUsesOneNameInputAndReturnsToRenamedProject(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		t.Run(map[bool]string{false: "keyboard", true: "mouse"}[mouse], func(t *testing.T) {
			service := &fakeService{projectPreview: app.ProjectEditPreview{}}
			service.snapshot = testSnapshot()
			service.snapshot.Config.Projects[0].Name = "renamed"
			m, cmd := openProjectMoreAction(t, projectWorkspaceModel(service), "Rename project", mouse)
			if cmd != nil || m.Mode != ModeInput || m.Input.Prompt != "Project name" {
				t.Fatalf("rename entry mode=%s prompt=%q cmd=%v", m.Mode, m.Input.Prompt, cmd != nil)
			}
			m = enterProjectText(m, "renamed")
			next, preview := m.Update(actionKey(tea.KeyEnter))
			m = next.(Model)
			if preview == nil {
				t.Fatal("rename preview was not deferred")
			}
			next, _ = m.Update(preview())
			m = next.(Model)
			wantPreview := app.ProjectEditRequest{Project: "aikit", Name: "renamed"}
			if !reflect.DeepEqual(service.lastProjectPreview, wantPreview) || m.Mode != ModeConfirm {
				t.Fatalf("rename preview request=%+v mode=%s", service.lastProjectPreview, m.Mode)
			}
			next, execute := m.Update(actionKey(tea.KeyEnter))
			m = next.(Model)
			operation := execute()
			if service.lastProjectEdit.Project != "aikit" || service.lastProjectEdit.Name != "renamed" || !service.lastProjectEdit.Confirmed {
				t.Fatalf("rename request=%+v", service.lastProjectEdit)
			}
			next, refresh := m.Update(operation)
			m = next.(Model)
			next, _ = m.Update(refresh())
			m = next.(Model)
			if m.Scope != (Scope{Project: "renamed", Level: "project-targets"}) {
				t.Fatalf("rename route=%+v", m.Scope)
			}
		})
	}
}

func TestProjectPathChangeUsesOnePathInputAndOpaqueIdentity(t *testing.T) {
	service := &fakeService{projectPreview: app.ProjectEditPreview{PathIdentity: "replacement-token"}}
	m, cmd := openProjectMoreAction(t, projectWorkspaceModel(service), "Change project directory", false)
	if cmd != nil || m.Mode != ModeInput || m.Input.Prompt != "Project directory" {
		t.Fatalf("path entry mode=%s prompt=%q cmd=%v", m.Mode, m.Input.Prompt, cmd != nil)
	}
	m = enterProjectText(m, "/work/moved")
	next, preview := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	next, _ = m.Update(preview())
	m = next.(Model)
	if !reflect.DeepEqual(service.lastProjectPreview, app.ProjectEditRequest{Project: "aikit", Path: "/work/moved"}) {
		t.Fatalf("path preview request=%+v", service.lastProjectPreview)
	}
	if strings.Contains(m.ViewString(), "replacement-token") {
		t.Fatalf("opaque identity rendered:\n%s", m.ViewString())
	}
	next, execute := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	_ = execute()
	if service.lastProjectEdit.ExpectedPathIdentity != "replacement-token" || service.lastProjectEdit.Path != "/work/moved" {
		t.Fatalf("path edit request=%+v", service.lastProjectEdit)
	}
}

func TestProjectAgentChecklistSavesOneExactDiff(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		t.Run(map[bool]string{false: "keyboard", true: "mouse"}[mouse], func(t *testing.T) {
			service := &fakeService{projectPreview: app.ProjectEditPreview{}}
			m := projectWorkspaceModel(service)
			m.Scope = Scope{Project: "aikit", Level: "project-targets"}
			m, cmd := openProjectMoreAction(t, m, "Manage agents", mouse)
			if cmd != nil || m.Mode != ModeProjectAgents || len(m.rows()) != len(agent.Names()) || !m.Selected["project-agent:codex"] {
				t.Fatalf("agent checklist mode=%s rows=%d selected=%v cmd=%v", m.Mode, len(m.rows()), m.Selected, cmd != nil)
			}
			cursor := -1
			for i, current := range m.rows() {
				if current.ID == "cursor" {
					cursor = i
				}
			}
			if cursor < 0 {
				t.Fatal("cursor agent missing from checklist")
			}
			m.Cursor = cursor
			next, _ := m.Update(actionKey(tea.KeySpace))
			m = next.(Model)
			save := actionIndex(t, m, "Save agents")
			if mouse {
				m, cmd = mouseAction(t, m, save)
			} else {
				m, cmd = keyboardAction(t, m, save)
			}
			if cmd == nil || service.projectPreviewCalls != 0 {
				t.Fatalf("agent preview cmd=%v calls=%d", cmd != nil, service.projectPreviewCalls)
			}
			next, _ = m.Update(cmd())
			m = next.(Model)
			want := app.ProjectEditRequest{Project: "aikit", AddAgents: []string{"cursor"}}
			if !reflect.DeepEqual(service.lastProjectPreview, want) || m.Mode != ModeConfirm {
				t.Fatalf("agent diff=%+v want=%+v mode=%s", service.lastProjectPreview, want, m.Mode)
			}
			cancelled, cancelCmd := m.Update(actionKey(tea.KeyEsc))
			if cancelCmd != nil || service.projectEditCalls != 0 || cancelled.(Model).Mode == ModeConfirm {
				t.Fatal("agent confirmation cancel mutated")
			}
		})
	}
}

func TestEmptyProjectKeepsCommonAndManageAgentsDiscoverable(t *testing.T) {
	service := &fakeService{}
	m := projectWorkspaceModel(service)
	m.Snapshot.Config.Projects = []config.Project{{Name: "empty", Path: "/work/empty", AgentBindings: map[string]config.Binding{}}}
	m.Scope = Scope{Project: "empty", Level: "project-targets"}
	rows := m.rows()
	if len(rows) != 1 || rows[0].ID != "common" || !strings.Contains(m.ViewString(), "No agents configured") {
		t.Fatalf("empty project rows=%+v\n%s", rows, m.ViewString())
	}
	if index := actionIndex(t, m, "Manage agents"); index < 0 {
		t.Fatal("empty project has no Manage agents action")
	}
}
