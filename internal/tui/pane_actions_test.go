package tui

import (
	"context"
	"reflect"
	"testing"
)

func TestPaneActionOwnership(t *testing.T) {
	tests := []struct {
		name       string
		view       View
		scope      Scope
		configure  func(*Model)
		collection []string
		detail     []string
	}{
		{name: "library", view: ViewLibrary, collection: []string{"Add source", "More"}, detail: []string{"Open", "More"}},
		{name: "global", view: ViewWorkspaces, scope: Scope{Level: "workspace-global"}, collection: []string{"Sync preview"}, detail: []string{"Open", "Apply preset"}},
		{name: "agents", view: ViewWorkspaces, scope: Scope{Level: "workspace-agents"}, collection: []string{"Sync preview"}, detail: []string{"Open", "Apply preset"}},
		{name: "projects", view: ViewWorkspaces, scope: Scope{Level: "workspace-projects"}, collection: []string{"Create project"}, detail: []string{"Add skill", "Apply preset", "More"}},
		{name: "presets", view: ViewPresets, collection: []string{"Create preset"}, detail: []string{"Edit members", "Apply", "More"}},
		{name: "status", view: ViewStatus, collection: []string{"Refresh"}, detail: []string{"Open", "Adopt", "More"}},
		{name: "migration", view: ViewMigration, configure: func(m *Model) {
			m.Inventory.Items = testInventoryItems()
		}, collection: []string{"Refresh"}, detail: []string{"Adopt", "Ignore"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, tt.view, ActionNone)
			m.Snapshot, m.Scope, m.Width, m.Height = testSnapshot(), tt.scope, 120, 24
			if tt.configure != nil {
				tt.configure(&m)
			}
			if got := m.collectionActions(); !reflect.DeepEqual(got, tt.collection) {
				t.Fatalf("collection actions=%v want=%v", got, tt.collection)
			}
			if got := m.detailActions(); !reflect.DeepEqual(got, tt.detail) {
				t.Fatalf("detail actions=%v want=%v", got, tt.detail)
			}
		})
	}
}

func TestPaneMoreActionsStayOwned(t *testing.T) {
	library := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	library.Snapshot = testSnapshot()
	library.Selected["library:acme/alpha"] = true
	collection := library.moreActions(actionPaneCollection)
	detail := library.moreActions(actionPaneDetail)
	for _, wanted := range []string{"State filter", "Source filter", "Check updates", "Enable selected", "Remove selected", "Clear selection"} {
		if !containsString(collection, wanted) {
			t.Fatalf("collection More missing %q: %v", wanted, collection)
		}
	}
	for _, forbidden := range []string{"Change ref", "Remove"} {
		if containsString(collection, forbidden) {
			t.Fatalf("collection More contains detail action %q: %v", forbidden, collection)
		}
	}
	if !reflect.DeepEqual(detail, []string{"Change ref", "Remove", "Close"}) {
		t.Fatalf("library detail More=%v", detail)
	}

	project := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewWorkspaces, ActionNone)
	project.Snapshot, project.Scope = testSnapshot(), Scope{Level: "workspace-projects"}
	projectMore := project.moreActions(actionPaneDetail)
	for _, wanted := range []string{"Rename project", "Manage agents", "Change project directory", "Remove project"} {
		if !containsString(projectMore, wanted) {
			t.Fatalf("project detail More missing %q: %v", wanted, projectMore)
		}
	}
	if containsString(projectMore, "Create project") {
		t.Fatalf("project detail More contains collection action: %v", projectMore)
	}

	preset := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewPresets, ActionNone)
	preset.Snapshot = testSnapshot()
	presetMore := preset.moreActions(actionPaneDetail)
	for _, wanted := range []string{"Duplicate", "Rename", "Delete"} {
		if !containsString(presetMore, wanted) {
			t.Fatalf("preset detail More missing %q: %v", wanted, presetMore)
		}
	}
	if containsString(presetMore, "Create preset") {
		t.Fatalf("preset detail More contains collection action: %v", presetMore)
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestEmptyCollectionsKeepCreateActionsWithoutDetailActions(t *testing.T) {
	tests := []struct {
		name       string
		view       View
		scope      Scope
		collection []string
		clear      func(*Model)
	}{
		{name: "library", view: ViewLibrary, collection: []string{"Add source", "More"}, clear: func(m *Model) {
			m.Snapshot.Config.Library.Skills = nil
		}},
		{name: "projects", view: ViewWorkspaces, scope: Scope{Level: "workspace-projects"}, collection: []string{"Create project"}, clear: func(m *Model) {
			m.Snapshot.Config.Projects = nil
		}},
		{name: "presets", view: ViewPresets, collection: []string{"Create preset"}, clear: func(m *Model) {
			m.Snapshot.Config.Presets = nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, tt.view, ActionNone)
			m.Snapshot, m.Scope = testSnapshot(), tt.scope
			tt.clear(&m)
			if got := m.collectionActions(); !reflect.DeepEqual(got, tt.collection) {
				t.Fatalf("collection actions=%v want=%v", got, tt.collection)
			}
			if got := m.detailActions(); len(got) != 0 {
				t.Fatalf("empty detail actions=%v", got)
			}
		})
	}
}
