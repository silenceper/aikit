package tui

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func projectWorkspaceModel(service *fakeService) Model {
	m := NewModel(nil, service, &fakeMigration{}, ViewWorkspaces, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 110, 30
	m.Scope = Scope{Level: "workspace-projects"}
	return m
}

func invokeProjectAction(t *testing.T, m Model, label string, mouse bool) (Model, tea.Cmd) {
	t.Helper()
	if label == "Create project" {
		if mouse {
			return mouseAction(t, m, actionIndex(t, m, label))
		}
		return keyboardAction(t, m, actionIndex(t, m, label))
	}
	if mouse {
		m, _ = mouseAction(t, m, actionIndex(t, m, "More"))
	} else {
		m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
	}
	index := actionIndex(t, m, label)
	if mouse {
		region := m.hitRegions().Actions[index]
		next, cmd := m.Update(click(region.X, region.Y))
		return next.(Model), cmd
	}
	for m.ActionIndex != index {
		next, _ := m.Update(actionKey(tea.KeyRight))
		m = next.(Model)
	}
	next, cmd := m.Update(actionKey(tea.KeyEnter))
	return next.(Model), cmd
}

func enterProjectText(m Model, value string) Model {
	for _, char := range value {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		m = next.(Model)
	}
	return m
}

func TestProjectDirectoryRebindPreviewConfirmKeyboardMouse(t *testing.T) {
	tests := []struct {
		name, action, input string
		want                app.ProjectEditRequest
	}{
		{"edit-rebind", "Change project directory", "/work/moved", app.ProjectEditRequest{Project: "aikit", Path: "/work/moved"}},
	}
	for _, tt := range tests {
		for _, mouse := range []bool{false, true} {
			t.Run(tt.name+map[bool]string{false: "-keyboard", true: "-mouse"}[mouse], func(t *testing.T) {
				service := &fakeService{projectPreview: app.ProjectEditPreview{
					Cleanup: link.Plan{Actions: []link.Action{{Path: "/work/aikit/.codex/skills/alpha"}}, Warnings: []link.Issue{{Path: "/work/aikit/custom", Message: "unknown content is preserved"}}},
					Next:    link.Plan{Actions: []link.Action{{Path: "/work/moved/.cursor/skills/alpha"}}},
				}}
				m, cmd := invokeProjectAction(t, projectWorkspaceModel(service), tt.action, mouse)
				if cmd != nil || m.Mode != ModeInput || service.projectPreviewCalls != 0 || service.projectEditCalls != 0 {
					t.Fatalf("entry mode=%s cmd=%v preview=%d edit=%d", m.Mode, cmd != nil, service.projectPreviewCalls, service.projectEditCalls)
				}
				m = enterProjectText(m, tt.input)
				next, previewCmd := m.Update(actionKey(tea.KeyEnter))
				m = next.(Model)
				if previewCmd == nil || service.projectPreviewCalls != 0 {
					t.Fatalf("preview not deferred: cmd=%v calls=%d", previewCmd != nil, service.projectPreviewCalls)
				}
				next, _ = m.Update(previewCmd())
				m = next.(Model)
				if m.Mode != ModeConfirm || service.projectPreviewCalls != 1 || service.projectEditCalls != 0 || !reflect.DeepEqual(service.lastProjectPreview, tt.want) {
					t.Fatalf("preview mode=%s request=%+v want=%+v", m.Mode, service.lastProjectPreview, tt.want)
				}
				for _, wanted := range []string{"unknown content is preserved", "/work/aikit/.codex/skills/alpha", "/work/moved/.cursor/skills/alpha"} {
					if !strings.Contains(m.ViewString(), wanted) {
						t.Fatalf("project preview missing %q:\n%s", wanted, m.ViewString())
					}
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
				_, duplicate := m.Update(actionKey(tea.KeyEnter))
				if duplicate != nil {
					t.Fatal("busy project edit accepted duplicate submit")
				}
				_ = execute()
				if service.projectEditCalls != 1 || !service.lastProjectEdit.Confirmed {
					t.Fatalf("edit calls=%d request=%+v", service.projectEditCalls, service.lastProjectEdit)
				}
			})
		}
	}
}

func TestProjectRemoveResultKeepsUnknownWarningsVisible(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		t.Run(map[bool]string{false: "keyboard", true: "mouse"}[mouse], func(t *testing.T) {
			service := &fakeService{
				projectRemovePreview: app.MutationPreview{Summary: `Remove project "aikit"`, AffectedScopes: []config.Scope{{Project: "aikit", Agent: "codex"}}, Plan: link.Plan{Actions: []link.Action{{Path: "/work/aikit/.codex/skills/alpha"}}}, RequiresConfirmation: true},
				projectRemoveResult:  app.Result{Changed: true, Warnings: []string{"unknown entries were preserved"}, Link: link.Result{Warnings: []link.Issue{{Path: "/work/aikit/custom", Message: "not managed; preserved"}}}},
			}
			m, preview := invokeProjectAction(t, projectWorkspaceModel(service), "Remove project", mouse)
			if preview == nil || m.Mode == ModeConfirm || service.projectRemovePreviewCalls != 0 || service.projectRemoveCalls != 0 {
				t.Fatalf("remove preview cmd=%v mode=%s previewCalls=%d removeCalls=%d", preview != nil, m.Mode, service.projectRemovePreviewCalls, service.projectRemoveCalls)
			}
			next, _ := m.Update(preview())
			m = next.(Model)
			if m.Mode != ModeConfirm || service.projectRemovePreviewCalls != 1 || service.lastProjectRemovePreview.Project != "aikit" || !strings.Contains(m.ViewString(), "/work/aikit/.codex/skills/alpha") {
				t.Fatalf("remove preview not visible/typed: mode=%s request=%+v\n%s", m.Mode, service.lastProjectRemovePreview, m.ViewString())
			}
			next, execute := m.Update(actionKey(tea.KeyEnter))
			m = next.(Model)
			if execute == nil || !m.MutationBusy || service.projectRemoveCalls != 0 {
				t.Fatalf("remove not deferred: cmd=%v busy=%v calls=%d", execute != nil, m.MutationBusy, service.projectRemoveCalls)
			}
			next, _ = m.Update(execute())
			m = next.(Model)
			for _, wanted := range []string{"unknown entries were preserved", "/work/aikit/custom", "not managed; preserved"} {
				if !strings.Contains(m.ViewString(), wanted) {
					t.Fatalf("remove result missing %q:\n%s", wanted, m.ViewString())
				}
			}
		})
	}
}

func TestProjectRemoveShortcutAndMouseActionSharePreviewConfirmFlow(t *testing.T) {
	run := func(t *testing.T, shortcut bool) (*fakeService, Model) {
		t.Helper()
		service := &fakeService{projectRemovePreview: app.MutationPreview{Summary: `Remove project "aikit"`, Plan: link.Plan{Actions: []link.Action{{Path: "/work/aikit/.codex/skills/alpha"}}}, RequiresConfirmation: true}}
		m := projectWorkspaceModel(service)
		var preview tea.Cmd
		if shortcut {
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
			m, preview = next.(Model), cmd
		} else {
			m, preview = invokeProjectAction(t, m, "Remove project", true)
		}
		if preview == nil || service.projectRemovePreviewCalls != 0 || service.projectRemoveCalls != 0 {
			t.Fatalf("preview cmd=%v previewCalls=%d removeCalls=%d", preview != nil, service.projectRemovePreviewCalls, service.projectRemoveCalls)
		}
		next, _ := m.Update(preview())
		m = next.(Model)
		if m.Mode != ModeConfirm || service.projectRemovePreviewCalls != 1 || service.lastProjectRemovePreview.Project != "aikit" {
			t.Fatalf("typed preview mode=%s calls=%d request=%+v", m.Mode, service.projectRemovePreviewCalls, service.lastProjectRemovePreview)
		}
		cancelled, cancelCmd := m.Update(actionKey(tea.KeyEsc))
		if cancelCmd != nil || service.projectRemoveCalls != 0 || cancelled.(Model).Mode == ModeConfirm {
			t.Fatal("cancelled remove mutated")
		}
		next, execute := m.Update(actionKey(tea.KeyEnter))
		m = next.(Model)
		if execute == nil || service.projectRemoveCalls != 0 || !m.MutationBusy {
			t.Fatal("remove confirmation was not deferred")
		}
		_ = execute()
		if service.projectRemoveCalls != 1 || service.lastProjectRemove.Project != "aikit" || !service.lastProjectRemove.Confirmed {
			t.Fatalf("remove request=%+v calls=%d", service.lastProjectRemove, service.projectRemoveCalls)
		}
		return service, m
	}
	shortcutService, _ := run(t, true)
	mouseService, _ := run(t, false)
	if !reflect.DeepEqual(shortcutService.lastProjectRemovePreview, mouseService.lastProjectRemovePreview) || !reflect.DeepEqual(shortcutService.lastProjectRemove, mouseService.lastProjectRemove) {
		t.Fatalf("shortcut/mouse mismatch preview=%+v/%+v remove=%+v/%+v", shortcutService.lastProjectRemovePreview, mouseService.lastProjectRemovePreview, shortcutService.lastProjectRemove, mouseService.lastProjectRemove)
	}
}

func TestProjectPartialResultAndIssueErrorRemainVisible(t *testing.T) {
	m := projectWorkspaceModel(&fakeService{})
	issueErr := errors.New("unlink permission denied")
	result := app.Result{Changed: true, Exit: app.ExitPartial, Link: link.Result{Issues: []link.Issue{{Path: "/work/aikit/.codex/skills/alpha", Message: "cleanup failed", Err: issueErr}}}}
	next, cmd := m.Update(projectOperationMsg{name: "remove project", result: result})
	m = next.(Model)
	if cmd == nil || !strings.Contains(strings.ToLower(m.Status), "partial") || strings.Contains(strings.ToLower(m.Status), "completed") {
		t.Fatalf("partial result status=%q cmd=%v", m.Status, cmd != nil)
	}
	for _, wanted := range []string{"/work/aikit/.codex/skills/alpha", "cleanup failed", "unlink permission denied"} {
		if !strings.Contains(m.ViewString(), wanted) {
			t.Fatalf("partial result missing %q:\n%s", wanted, m.ViewString())
		}
	}
}

func TestConfirmOverlayScrollKeyboardAndMouseInspectEveryPlanItem(t *testing.T) {
	m := projectWorkspaceModel(&fakeService{})
	m.Height = 12
	m.Mode, m.confirm = ModeConfirm, ActionRemoveProject
	m.Preview.Title, m.Preview.Summary = "Remove project", `Remove project "aikit" and clean its managed paths`
	for i := 0; i < 18; i++ {
		m.Preview.Plan.Actions = append(m.Preview.Plan.Actions, link.Action{Path: fmt.Sprintf("/cleanup/%02d", i)})
	}
	initial := m.ViewString()
	if !strings.Contains(initial, `Remove project "aikit"`) || strings.Contains(initial, "/cleanup/17") || !strings.Contains(initial, "/19") {
		t.Fatalf("initial confirm viewport invalid:\n%s", initial)
	}
	for i := 0; i < 18; i++ {
		next, _ := m.Update(actionKey(tea.KeyDown))
		m = next.(Model)
	}
	if !strings.Contains(m.ViewString(), "/cleanup/17") {
		t.Fatalf("keyboard could not inspect final plan item:\n%s", m.ViewString())
	}
	m.OverlayScroll = 0
	layout := layoutOverlayPanel(ComputeLayout(m.Width, m.Height), m.overlayPanelActions(), true, m.ActionIndex)
	for i := 0; i < 18; i++ {
		next, _ := m.Update(tea.MouseMsg{X: layout.Body.X, Y: layout.Body.Y, Button: tea.MouseButtonWheelDown})
		m = next.(Model)
	}
	if !strings.Contains(m.ViewString(), "/cleanup/17") {
		t.Fatalf("mouse wheel could not inspect final plan item:\n%s", m.ViewString())
	}
}
