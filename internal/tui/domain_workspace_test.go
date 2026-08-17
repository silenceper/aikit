package tui

import (
	"context"
	"reflect"
	"testing"
)

func TestProjectAndPresetPrimaryActionsAreTaskOriented(t *testing.T) {
	project := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewWorkspaces, ActionNone)
	project.Snapshot, project.Scope = testSnapshot(), Scope{Level: "workspace-projects"}
	if got := project.primaryActions(); !reflect.DeepEqual(got, []string{"Add skill", "Apply preset", "More"}) {
		t.Fatalf("project actions=%v", got)
	}

	preset := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewPresets, ActionNone)
	preset.Snapshot = testSnapshot()
	if got := preset.primaryActions(); !reflect.DeepEqual(got, []string{"Edit members", "Apply", "More"}) {
		t.Fatalf("preset actions=%v", got)
	}
}

func TestProjectAddSkillChoosesExactTargetBeforePicker(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewWorkspaces, ActionNone)
	m.Snapshot, m.Scope = testSnapshot(), Scope{Level: "workspace-projects"}
	next, cmd := m.performPrimaryAction(0)
	m = next.(Model)
	if cmd != nil || m.Scope.Level != "project-targets" || m.workspaceIntent != "add-skill" {
		t.Fatalf("add skill did not request target: scope=%+v intent=%q cmd=%v", m.Scope, m.workspaceIntent, cmd != nil)
	}
	next, cmd = m.perform(uiActivate)
	m = next.(Model)
	if cmd != nil || m.Mode != ModeWorkspaceSkills || m.Scope.Level != "project-skills" {
		t.Fatalf("target did not open skill picker: mode=%s scope=%+v cmd=%v", m.Mode, m.Scope, cmd != nil)
	}
}
