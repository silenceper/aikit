package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func TestProjectRegistrationPathOnlyKeyboardMouse(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		t.Run(map[bool]string{false: "keyboard", true: "mouse"}[mouse], func(t *testing.T) {
			created := config.Project{Name: "aikit-new", Path: "/canonical/aikit", Agents: []string{"cursor", "codex"}, AgentBindings: map[string]config.Binding{}}
			service := &fakeService{
				projectRegistration: app.ProjectRegistrationPreview{
					Name: "aikit-new", Path: "/canonical/aikit", PathIdentity: "opaque-v1-token", Agents: []string{"cursor", "codex"},
					Warnings: []string{"windsurf integration is not a safe directory"},
					Preview: app.ProjectEditPreview{
						PathIdentity: "opaque-v1-token",
						Next:         link.Plan{Actions: []link.Action{{Path: "/canonical/aikit/.cursor/skills/alpha"}}},
					},
				},
			}
			service.snapshot = testSnapshot()
			service.snapshot.Config.Projects = append(service.snapshot.Config.Projects, created)
			m, cmd := invokeProjectAction(t, projectWorkspaceModel(service), "Create project", mouse)
			if cmd != nil || m.Mode != ModeInput || m.Input.Prompt != "Project directory" {
				t.Fatalf("create entry mode=%s prompt=%q cmd=%v", m.Mode, m.Input.Prompt, cmd != nil)
			}
			m = enterProjectText(m, "/work/aikit")
			next, previewCmd := m.Update(actionKey(tea.KeyEnter))
			m = next.(Model)
			if previewCmd == nil || service.projectRegistrationCalls != 0 {
				t.Fatalf("registration preview not deferred: cmd=%v calls=%d", previewCmd != nil, service.projectRegistrationCalls)
			}
			next, _ = m.Update(previewCmd())
			m = next.(Model)
			if service.projectRegistrationCalls != 1 || service.lastProjectRegistration != (app.ProjectRegistrationRequest{Path: "/work/aikit"}) || m.Mode != ModeConfirm {
				t.Fatalf("preview request=%+v calls=%d mode=%s", service.lastProjectRegistration, service.projectRegistrationCalls, m.Mode)
			}
			view := m.ViewString()
			for _, want := range []string{"aikit-new", "/canonical/aikit", "cursor", "codex", "windsurf integration", "/canonical/aikit/.cursor/skills/alpha"} {
				if !strings.Contains(view, want) {
					t.Fatalf("registration confirmation missing %q:\n%s", want, view)
				}
			}
			if strings.Contains(view, "opaque-v1-token") || strings.Contains(view, "pipe-separated") {
				t.Fatalf("confirmation leaked implementation token/encoded input:\n%s", view)
			}
			cancelled, cancelCmd := m.Update(actionKey(tea.KeyEsc))
			if cancelCmd != nil || service.projectEditCalls != 0 || cancelled.(Model).Mode == ModeConfirm {
				t.Fatalf("cancel mutated: cmd=%v calls=%d", cancelCmd != nil, service.projectEditCalls)
			}

			next, execute := m.Update(actionKey(tea.KeyEnter))
			m = next.(Model)
			if execute == nil || service.projectEditCalls != 0 || !m.MutationBusy {
				t.Fatalf("confirm not deferred: cmd=%v calls=%d busy=%v", execute != nil, service.projectEditCalls, m.MutationBusy)
			}
			operation := execute()
			wantRequest := app.ProjectEditRequest{
				Name: "aikit-new", Path: "/canonical/aikit", AddAgents: []string{"cursor", "codex"}, ExpectedPathIdentity: "opaque-v1-token", Confirmed: true,
			}
			if !reflect.DeepEqual(service.lastProjectEdit, wantRequest) {
				t.Fatalf("edit request=%+v want=%+v", service.lastProjectEdit, wantRequest)
			}
			next, refresh := m.Update(operation)
			m = next.(Model)
			if refresh == nil {
				t.Fatal("successful project creation did not refresh snapshot")
			}
			next, _ = m.Update(refresh())
			m = next.(Model)
			if m.ActiveView != ViewWorkspaces || m.Scope.Level != "project-targets" || m.Scope.Project != "aikit-new" {
				t.Fatalf("post-create route view=%s scope=%+v", m.ActiveView, m.Scope)
			}
		})
	}
}

func TestProjectRegistrationNameConflictUsesSeparateNameInput(t *testing.T) {
	service := &fakeService{}
	service.projectRegistrationFunc = func(request app.ProjectRegistrationRequest) app.ProjectRegistrationPreview {
		if request.Name == "" {
			return app.ProjectRegistrationPreview{Name: "aikit", Path: "/canonical/aikit", PathIdentity: "token", NeedsName: true, NameIssue: app.ProjectNameDuplicate}
		}
		return app.ProjectRegistrationPreview{
			Name: request.Name, Path: "/canonical/aikit", PathIdentity: "token", Agents: []string{"codex"},
			Preview: app.ProjectEditPreview{PathIdentity: "token"},
		}
	}
	m, _ := invokeProjectAction(t, projectWorkspaceModel(service), "Create project", false)
	m = enterProjectText(m, "/work/aikit")
	next, previewCmd := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if previewCmd == nil {
		t.Fatalf("registration preview was not deferred; mode=%s prompt=%q err=%q", m.Mode, m.Input.Prompt, m.Err)
	}
	next, _ = m.Update(previewCmd())
	m = next.(Model)
	if m.Mode != ModeInput || m.Input.Prompt != "Project name" || !strings.Contains(strings.ToLower(m.Status), "already") {
		t.Fatalf("name conflict mode=%s prompt=%q status=%q", m.Mode, m.Input.Prompt, m.Status)
	}
	m = enterProjectText(m, "aikit-work")
	next, namedPreview := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if namedPreview == nil {
		t.Fatal("named registration preview was not deferred")
	}
	next, _ = m.Update(namedPreview())
	m = next.(Model)
	if service.projectRegistrationCalls != 2 || service.lastProjectRegistration != (app.ProjectRegistrationRequest{Path: "/canonical/aikit", Name: "aikit-work"}) || m.Mode != ModeConfirm {
		t.Fatalf("named preview request=%+v calls=%d mode=%s", service.lastProjectRegistration, service.projectRegistrationCalls, m.Mode)
	}
}
