package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/pkg/config"
)

func mouseActionByLabel(t *testing.T, m Model, label string) (Model, tea.Cmd) {
	t.Helper()
	wanted := actionIndex(t, m, label)
	for attempts := 0; attempts < len(m.currentActions())+2; attempts++ {
		regions := m.hitRegions()
		for visible, absolute := range regions.ActionIndexes {
			if absolute == wanted {
				next, cmd := m.Update(click(regions.Actions[visible].X, regions.Actions[visible].Y))
				return next.(Model), cmd
			}
		}
		if regions.ActionNext.Empty() {
			t.Fatalf("action %q is not mouse reachable: indexes=%v", label, regions.ActionIndexes)
		}
		next, _ := m.Update(click(regions.ActionNext.X, regions.ActionNext.Y))
		m = next.(Model)
	}
	t.Fatalf("action %q did not become visible", label)
	return m, nil
}

func rowByID(t *testing.T, rows []row, id string) row {
	t.Helper()
	for _, current := range rows {
		if current.ID == id {
			return current
		}
	}
	t.Fatalf("row %q missing: %+v", id, rows)
	return row{}
}

func TestWorkspaceProjectionShowsEveryOwnerAndAvoidsMisleadingDisable(t *testing.T) {
	snapshot := testSnapshot()
	snapshot.Config.Agents["codex"] = config.Binding{Skills: []string{"acme/alpha"}}
	snapshot.Config.Projects[0].Binding = config.Binding{Presets: []string{"review"}}
	snapshot.Config.Projects[0].AgentBindings = map[string]config.Binding{"codex": {Skills: []string{"acme/alpha"}}}
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewWorkspaces, ActionNone)
	m.Snapshot, m.Scope = snapshot, Scope{Project: "aikit", Agent: "codex", Level: "project-skills"}
	projectRow := rowByID(t, m.rows(), "acme/alpha")
	for _, owner := range []string{"Direct", "Preset: review", "Project common", "Global inherited"} {
		if !strings.Contains(projectRow.Detail, owner) {
			t.Fatalf("project provenance missing %q: %+v", owner, projectRow)
		}
	}
	if !projectRow.Direct || !projectRow.Enabled {
		t.Fatalf("direct project row=%+v", projectRow)
	}

	// A preset-only effective skill is enabled, but removing a nonexistent
	// direct binding cannot disable it. The visible action must not promise so.
	m.Snapshot.Config.Agents["codex"] = config.Binding{Presets: []string{"review"}}
	m.Scope = Scope{Agent: "codex", Level: "agent-skills"}
	m.Cursor = 0
	agentRow := rowByID(t, m.rows(), "acme/alpha")
	if !agentRow.Enabled || agentRow.Direct || !strings.Contains(agentRow.Detail, "Preset: review") {
		t.Fatalf("preset-only row=%+v", agentRow)
	}
	m.Cursor = 0
	if strings.Contains(strings.Join(m.primaryActions(), " "), "Disable") {
		t.Fatalf("preset-only row advertises misleading disable: %v", m.primaryActions())
	}
}

func TestWorkspaceProjectionMixedSourcesCountsAndConflicts(t *testing.T) {
	snapshot := testSnapshot()
	snapshot.Config.Agents["codex"] = config.Binding{Skills: []string{"acme/alpha"}, Presets: []string{"review"}}
	snapshot.Config.Agents["cursor"] = config.Binding{Presets: []string{"review"}}
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewWorkspaces, ActionNone)
	m.Snapshot, m.Scope = snapshot, Scope{Agent: "codex", Level: "agent-skills"}
	alpha := rowByID(t, m.rows(), "acme/alpha")
	for _, owner := range []string{"Direct", "Preset: review"} {
		if !strings.Contains(alpha.Detail, owner) {
			t.Fatalf("mixed global owner %q missing: %+v", owner, alpha)
		}
	}

	m.Scope = Scope{Level: "workspace-global"}
	alpha = rowByID(t, m.rows(), "acme/alpha")
	if alpha.State != "2/5 agents" || !strings.Contains(alpha.Detail, "codex: Direct + Preset: review") || !strings.Contains(alpha.Detail, "cursor: Preset: review") {
		t.Fatalf("aggregate global projection=%+v", alpha)
	}

	snapshot.Config.Library.Skills = append(snapshot.Config.Library.Skills, config.Skill{ID: "other/alpha", Name: "alpha"})
	snapshot.Config.Presets = append(snapshot.Config.Presets, config.Preset{Name: "collision", Skills: []string{"other/alpha"}})
	snapshot.Config.Agents["codex"] = config.Binding{Skills: []string{"acme/alpha"}, Presets: []string{"collision"}}
	m.Snapshot, m.Scope = snapshot, Scope{Agent: "codex", Level: "agent-skills"}
	for _, id := range []string{"acme/alpha", "other/alpha"} {
		current := rowByID(t, m.rows(), id)
		if current.State != "Conflict" || current.Severity != rowSeverityConflict {
			t.Fatalf("conflict row %s=%+v", id, current)
		}
	}
}

func TestProjectTargetRowsReportEffectiveSkillsNotOnlyDirectEntries(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewWorkspaces, ActionNone)
	m.Snapshot, m.Scope = testSnapshot(), Scope{Project: "aikit", Level: "project-targets"}
	common := rowByID(t, m.rows(), "common")
	codex := rowByID(t, m.rows(), "codex")
	if common.State != "1 available" || codex.State != "2 available" {
		t.Fatalf("project target counts common=%+v codex=%+v", common, codex)
	}
	if !strings.Contains(codex.Detail, "Project common") || !strings.Contains(codex.Detail, "Global inherited") {
		t.Fatalf("project target provenance summary=%+v", codex)
	}
}

func TestWorkspaceAgentAndPresetSummariesExposeEffectiveUsage(t *testing.T) {
	snapshot := testSnapshot()
	snapshot.Config.Agents["codex"] = config.Binding{Presets: []string{"review"}}
	snapshot.Config.Projects[0].Presets = []string{"review"}
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewWorkspaces, ActionNone)
	m.Snapshot, m.Scope = snapshot, Scope{Level: "workspace-agents"}
	codex := rowByID(t, m.rows(), "codex")
	if codex.State != "1 available" || !strings.Contains(codex.Detail, "Preset: review") {
		t.Fatalf("agent summary=%+v", codex)
	}

	m.ActiveView, m.Scope = ViewPresets, Scope{}
	review := rowByID(t, m.rows(), "review")
	if review.State != "1 skill · 2 scopes" || !strings.Contains(review.Detail, "Global / codex") || !strings.Contains(review.Detail, "Project / aikit / Common") {
		t.Fatalf("preset usage=%+v", review)
	}

	m.ActiveView, m.Scope = ViewWorkspaces, Scope{Project: "aikit", Level: "project-targets"}
	m.enterMore()
	if !strings.Contains(strings.Join(m.primaryActions(), " "), "Apply preset") {
		t.Fatalf("project target More hides preset entry: %v", m.primaryActions())
	}
}

func TestWorkspaceSelectSkillsPreviewsOneExactScopeBatch(t *testing.T) {
	service := &fakeService{}
	m := NewModel(nil, service, &fakeMigration{}, ViewWorkspaces, ActionNone)
	m.Snapshot, m.Scope, m.Width, m.Height = testSnapshot(), Scope{Project: "aikit", Level: "project-skills"}, 110, 30
	m, cmd := chooseVisibleAction(t, m, "Select skills", false)
	if cmd != nil || m.Mode != ModeWorkspaceSkills {
		t.Fatalf("workspace picker mode=%s cmd=%v", m.Mode, cmd != nil)
	}
	m.Cursor = 1 // beta
	next, cmd := m.perform(uiToggle)
	m = next.(Model)
	if cmd != nil || !m.Selected["workspace-skill:acme/beta"] {
		t.Fatalf("workspace selection=%v cmd=%v", m.Selected, cmd != nil)
	}
	m, preview := chooseVisibleAction(t, m, "Enable selected", false)
	if preview == nil || service.previewBatchCalls != 0 {
		t.Fatalf("preview cmd=%v calls=%d", preview != nil, service.previewBatchCalls)
	}
	next, _ = m.Update(preview())
	m = next.(Model)
	want := app.BatchRequest{Operation: app.BatchEnable, Bindings: []app.BindingRequest{{SkillID: "acme/beta", Project: "aikit"}}}
	if m.Mode != ModeConfirm || !reflect.DeepEqual(service.lastBatchPreview, want) {
		t.Fatalf("workspace batch=%+v want=%+v mode=%s", service.lastBatchPreview, want, m.Mode)
	}
}

func TestWorkspaceSelectSkillsKeyboardMouseNarrowParity(t *testing.T) {
	tests := []struct {
		name  string
		scope Scope
		want  app.BindingRequest
	}{
		{"global-agent", Scope{Agent: "codex", Level: "agent-skills"}, app.BindingRequest{SkillID: "acme/beta", Agent: "codex"}},
		{"project-common", Scope{Project: "aikit", Level: "project-skills"}, app.BindingRequest{SkillID: "acme/beta", Project: "aikit"}},
		{"project-agent", Scope{Project: "aikit", Agent: "codex", Level: "project-skills"}, app.BindingRequest{SkillID: "acme/beta", Project: "aikit", Agent: "codex"}},
	}
	for _, tt := range tests {
		for _, mouse := range []bool{false, true} {
			t.Run(tt.name+map[bool]string{false: "-keyboard", true: "-mouse"}[mouse], func(t *testing.T) {
				service := &fakeService{}
				m := NewModel(nil, service, &fakeMigration{}, ViewWorkspaces, ActionNone)
				m.Snapshot, m.Scope, m.Width, m.Height = testSnapshot(), tt.scope, 38, 18
				var cmd tea.Cmd
				if mouse {
					m, cmd = mouseActionByLabel(t, m, "Select skills")
				} else {
					m, cmd = chooseVisibleAction(t, m, "Select skills", false)
				}
				if cmd != nil || m.Mode != ModeWorkspaceSkills {
					t.Fatalf("picker mode=%s cmd=%v", m.Mode, cmd != nil)
				}
				m.Cursor = 1
				if mouse {
					regions := m.hitRegions()
					next, toggleCmd := m.Update(click(regions.Checkboxes[1].X, regions.Checkboxes[1].Y))
					m, cmd = next.(Model), toggleCmd
				} else {
					next, toggleCmd := m.Update(actionKey(tea.KeySpace))
					m, cmd = next.(Model), toggleCmd
				}
				if cmd != nil || !m.Selected["workspace-skill:acme/beta"] {
					t.Fatalf("selection=%v cmd=%v", m.Selected, cmd != nil)
				}
				if mouse {
					m, cmd = mouseActionByLabel(t, m, "Enable selected")
				} else {
					m, cmd = chooseVisibleAction(t, m, "Enable selected", false)
				}
				if cmd == nil {
					t.Fatal("batch preview was not deferred")
				}
				_ = cmd()
				want := app.BatchRequest{Operation: app.BatchEnable, Bindings: []app.BindingRequest{tt.want}}
				if !reflect.DeepEqual(service.lastBatchPreview, want) {
					t.Fatalf("request=%+v want=%+v", service.lastBatchPreview, want)
				}
			})
		}
	}
}

func TestGlobalAgentApplyPresetUsesExactAgentWithoutEncodedInput(t *testing.T) {
	service := &fakeService{}
	m := NewModel(nil, service, &fakeMigration{}, ViewWorkspaces, ActionNone)
	m.Snapshot, m.Scope, m.Width, m.Height = testSnapshot(), Scope{Agent: "codex", Level: "agent-skills"}, 110, 30
	m, _ = chooseVisibleAction(t, m, "More", false)
	m, cmd := chooseVisibleAction(t, m, "Apply preset", false)
	if cmd != nil || m.Mode != ModePresetPicker {
		t.Fatalf("agent preset picker mode=%s cmd=%v", m.Mode, cmd != nil)
	}
	m, preview := chooseRowByName(t, m, "review", false)
	if preview == nil {
		t.Fatal("agent preset preview was not deferred")
	}
	_ = preview()
	want := app.PresetMutationRequest{Operation: app.PresetApply, Name: "review", Binding: app.BindingRequest{Preset: "review", Agent: "codex"}}
	if !reflect.DeepEqual(service.lastPresetMutationPreview, want) {
		t.Fatalf("agent preset request=%+v want=%+v", service.lastPresetMutationPreview, want)
	}
}
