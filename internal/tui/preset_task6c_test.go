package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func TestPresetConfirmRendersAllTypedDiagnostics(t *testing.T) {
	m := presetListModel(&fakeService{})
	m.Mode, m.confirm = ModeConfirm, ActionPreset
	m.Preview = app.MutationPreview{
		Warnings: []string{"top warning"}, Conflicts: []string{"top conflict"},
		Plan: link.Plan{
			Warnings: []link.Issue{{Path: "/warn/path", Message: "plan warning", Err: errors.New("warning cause")}},
			Issues:   []link.Issue{{Path: "/issue/path", Message: "plan issue", Err: errors.New("issue cause")}},
		},
	}
	view := m.ViewString()
	for _, wanted := range []string{"top warning", "top conflict", "/warn/path", "plan warning", "warning cause", "/issue/path", "plan issue", "issue cause"} {
		if !strings.Contains(view, wanted) {
			t.Fatalf("confirm diagnostics missing %q:\n%s", wanted, view)
		}
	}
}

func TestEmptyPresetsStillRenderMouseCreate(t *testing.T) {
	m := presetListModel(&fakeService{})
	m.Snapshot.Config.Presets = nil
	index := actionIndex(t, m, "Create preset")
	regions := m.hitRegions()
	if index >= len(regions.Actions) {
		t.Fatalf("empty preset Create has no mouse region: %+v", regions.Actions)
	}
	next, cmd := m.Update(click(regions.Actions[index].X, regions.Actions[index].Y))
	got := next.(Model)
	if cmd != nil || got.Mode != ModeInput || got.Input.Kind != inputPresetCreate {
		t.Fatalf("empty preset mouse Create mode=%s kind=%s cmd=%v", got.Mode, got.Input.Kind, cmd != nil)
	}
}

func TestNarrowEmptyCollectionActionBarsShareVisibleHitGeometry(t *testing.T) {
	tests := []struct {
		name  string
		model Model
		label string
	}{
		{"preset", presetListModel(&fakeService{}), "Create preset"},
		{"library", NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone), "Add source"},
		{"project", projectWorkspaceModel(&fakeService{}), "Create project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.model
			m.Width, m.Height, m.Detail = 38, 12, false
			switch m.ActiveView {
			case ViewPresets:
				m.Snapshot.Config.Presets = nil
			case ViewLibrary:
				m.Snapshot = testSnapshot()
				m.Snapshot.Config.Library.Skills = nil
			case ViewWorkspaces:
				m.Snapshot.Config.Projects = nil
			}
			view := m.ViewString()
			if !strings.Contains(view, "["+tt.label+"]") {
				t.Fatalf("narrow empty action is hidden:\n%s", view)
			}
			index := actionIndex(t, m, tt.label)
			region := m.hitRegions().Actions[index]
			for _, x := range []int{region.X, region.Right() - 1} {
				next, _ := m.Update(click(x, region.Y))
				if next.(Model).Mode != ModeInput {
					t.Fatalf("visible edge x=%d did not invoke %s", x, tt.label)
				}
			}
			if separatorX := region.Right(); separatorX < m.hitRegions().ActionBar.Right() {
				next, cmd := m.Update(click(separatorX, region.Y))
				if cmd != nil || next.(Model).Mode != m.Mode {
					t.Fatal("action separator/blank invoked an action")
				}
			}
		})
	}
}

func presetListModel(service *fakeService) Model {
	m := NewModel(nil, service, &fakeMigration{}, ViewPresets, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 110, 30
	return m
}

func invokePresetMore(t *testing.T, m Model, label string, mouse bool) (Model, tea.Cmd) {
	t.Helper()
	if mouse {
		m, _ = mouseAction(t, m, actionIndex(t, m, "More"))
		region := m.hitRegions().Actions[actionIndex(t, m, label)]
		next, cmd := m.Update(click(region.X, region.Y))
		return next.(Model), cmd
	}
	m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
	index := actionIndex(t, m, label)
	for m.ActionIndex != index {
		next, _ := m.Update(actionKey(tea.KeyRight))
		m = next.(Model)
	}
	next, cmd := m.Update(actionKey(tea.KeyEnter))
	return next.(Model), cmd
}

func TestPresetDuplicateRenameAndExactApplyPreviewConfirmParity(t *testing.T) {
	tests := []struct {
		label, input, target string
		want                 app.PresetMutationRequest
	}{
		{"Duplicate", "copy", "", app.PresetMutationRequest{Operation: app.PresetDuplicate, Name: "review", NewName: "copy"}},
		{"Rename", "renamed", "", app.PresetMutationRequest{Operation: app.PresetRename, Name: "review", NewName: "renamed"}},
		{"Apply", "global-codex", "Global / codex", app.PresetMutationRequest{Operation: app.PresetApply, Name: "review", Binding: app.BindingRequest{Preset: "review", Agent: "codex"}}},
		{"Apply", "project-common", "Project / aikit / Common", app.PresetMutationRequest{Operation: app.PresetApply, Name: "review", Binding: app.BindingRequest{Preset: "review", Project: "aikit"}}},
		{"Apply", "project-codex", "Project / aikit / codex", app.PresetMutationRequest{Operation: app.PresetApply, Name: "review", Binding: app.BindingRequest{Preset: "review", Project: "aikit", Agent: "codex"}}},
	}
	for _, tt := range tests {
		for _, mouse := range []bool{false, true} {
			t.Run(tt.label+"-"+tt.input+map[bool]string{false: "-keyboard", true: "-mouse"}[mouse], func(t *testing.T) {
				service := &fakeService{
					presetMutationPreview: app.MutationPreview{References: []string{"agent:codex"}, AffectedScopes: []config.Scope{{Agent: "codex"}}, Plan: link.Plan{Actions: []link.Action{{Path: "/work/aikit/.codex/skills/alpha"}}}, RequiresConfirmation: true},
				}
				m, cmd := invokePresetMore(t, presetListModel(service), tt.label, mouse)
				wantMode := ModeInput
				if tt.label == "Apply" {
					wantMode = ModeScopePicker
				}
				if cmd != nil || m.Mode != wantMode || service.mutatePresetCalls != 0 {
					t.Fatalf("entry mode=%s cmd=%v", m.Mode, cmd != nil)
				}
				var preview tea.Cmd
				if tt.label == "Apply" {
					m, preview = chooseRowByName(t, m, tt.target, mouse)
				} else {
					m = enterProjectText(m, tt.input)
					next, cmd := m.Update(actionKey(tea.KeyEnter))
					m, preview = next.(Model), cmd
				}
				if preview == nil || service.previewPresetMutationCalls != 0 {
					t.Fatalf("preview not deferred: cmd=%v calls=%d", preview != nil, service.previewPresetMutationCalls)
				}
				next, _ := m.Update(preview())
				m = next.(Model)
				if m.Mode != ModeConfirm || !strings.Contains(m.ViewString(), "Global / codex") || service.mutatePresetCalls != 0 {
					t.Fatalf("preview missing usage scope or mutated:\n%s", m.ViewString())
				}
				if tt.label == "Apply" && (service.previewPresetMutationCalls != 1 || !strings.Contains(m.ViewString(), "/work/aikit/.codex/skills/alpha")) {
					t.Fatalf("apply lacks exact mutation preview calls=%d:\n%s", service.previewPresetMutationCalls, m.ViewString())
				}
				if !reflect.DeepEqual(service.lastPresetMutationPreview, tt.want) {
					t.Fatalf("preset mutation preview=%+v want=%+v", service.lastPresetMutationPreview, tt.want)
				}
				cancelled, cancelCmd := m.Update(actionKey(tea.KeyEsc))
				if cancelCmd != nil || service.mutatePresetCalls != 0 || cancelled.(Model).Mode == ModeConfirm {
					t.Fatal("cancel mutated preset")
				}
				next, execute := m.Update(actionKey(tea.KeyEnter))
				m = next.(Model)
				if execute == nil || !m.MutationBusy || service.mutatePresetCalls != 0 {
					t.Fatal("confirm was not single deferred mutation")
				}
				_, duplicate := m.Update(actionKey(tea.KeyEnter))
				if duplicate != nil {
					t.Fatal("busy preset accepted duplicate submit")
				}
				_ = execute()
				want := tt.want
				want.Confirmed = true
				if service.mutatePresetCalls != 1 || !reflect.DeepEqual(service.lastPresetMutation, want) {
					t.Fatalf("mutation=%+v want=%+v", service.lastPresetMutation, want)
				}
			})
		}
	}
}

func TestPresetDeleteReferencesRequireSecondForceConfirmation(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		t.Run(map[bool]string{false: "keyboard", true: "mouse"}[mouse], func(t *testing.T) {
			service := &fakeService{presetMutationPreview: app.MutationPreview{References: []string{"agent:codex", "project:aikit:common"}, AffectedScopes: []config.Scope{{Agent: "codex"}, {Project: "aikit"}}, RequiresForce: true, RequiresConfirmation: true}}
			m, preview := invokePresetMore(t, presetListModel(service), "Delete", mouse)
			if preview == nil || service.previewPresetMutationCalls != 0 || service.mutatePresetCalls != 0 {
				t.Fatalf("delete preview cmd=%v preview=%d mutation=%d", preview != nil, service.previewPresetMutationCalls, service.mutatePresetCalls)
			}
			next, _ := m.Update(preview())
			m = next.(Model)
			if m.Mode != ModeConfirm || service.lastPresetMutationPreview.Operation != app.PresetDelete || !strings.Contains(m.ViewString(), "agent:codex") {
				t.Fatalf("delete preview=%+v\n%s", service.lastPresetMutationPreview, m.ViewString())
			}
			cancelled, cancelCmd := m.Update(actionKey(tea.KeyEsc))
			if cancelCmd != nil || service.mutatePresetCalls != 0 || cancelled.(Model).Mode == ModeConfirm {
				t.Fatal("cancelled delete mutated preset")
			}
			next, first := m.Update(actionKey(tea.KeyEnter))
			m = next.(Model)
			if first != nil || service.mutatePresetCalls != 0 || !strings.Contains(strings.ToLower(m.ViewString()), "force") {
				t.Fatalf("first confirmation authorized force: cmd=%v calls=%d", first != nil, service.mutatePresetCalls)
			}
			next, force := m.Update(actionKey(tea.KeyEnter))
			m = next.(Model)
			if force == nil || !m.MutationBusy || service.mutatePresetCalls != 0 {
				t.Fatal("second force confirmation did not defer mutation")
			}
			_ = force()
			if service.mutatePresetCalls != 1 || service.lastPresetMutation.Operation != app.PresetDelete || !service.lastPresetMutation.Force || !service.lastPresetMutation.Confirmed {
				t.Fatalf("delete mutation=%+v", service.lastPresetMutation)
			}
		})
	}
}
