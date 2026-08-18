package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPermanentNavigationAndToolsAreSeparated(t *testing.T) {
	want := []View{ViewOverview, ViewLibrary, ViewPresets, ViewStatus}
	if !reflect.DeepEqual(topViews, want) {
		t.Fatalf("topViews=%v want=%v", topViews, want)
	}
	entries := navigationEntries(Model{})
	if got := entryLabels(entries[:9]); !reflect.DeepEqual(got, []string{"Overview", "Library", "Presets", "Status", "Global", "Agents", "Projects", "Migration", "Configuration"}) {
		t.Fatalf("entries=%v", got)
	}
	for _, index := range []int{4, 5, 6} {
		if entries[index].Section != "Workspaces" || entries[index].Scope.Level == "" {
			t.Fatalf("workspace entry not direct: %+v", entries[index])
		}
	}
	if entries[7].Section != "Tools" || entries[8].Section != "Tools" {
		t.Fatalf("tools not separated: %+v", entries)
	}
}

func TestWorkspaceDirectRoutesKeyboardMouseAndAliases(t *testing.T) {
	service := &fakeService{}
	want := map[string]string{
		"Global":   "workspace-global",
		"Agents":   "workspace-agents",
		"Projects": "workspace-projects",
	}
	for label, scope := range want {
		t.Run(label, func(t *testing.T) {
			m := NewModel(context.Background(), service, &fakeMigration{}, ViewOverview, ActionNone)
			m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 24
			var entry navigationEntry
			for _, candidate := range navigationEntries(m) {
				if candidate.Label == label {
					entry = candidate
					break
				}
			}
			next, cmd := m.activateCommandEntry(entry)
			got := next.(Model)
			if cmd != nil || got.ActiveView != ViewWorkspaces || got.Scope.Level != scope {
				t.Fatalf("palette route view=%s scope=%s cmd=%v", got.ActiveView, got.Scope.Level, cmd != nil)
			}

			regions := m.hitRegions()
			var rect Rect
			for _, item := range regions.Navigation {
				if item.Entry.Label == label {
					rect = item.Rect
				}
			}
			if rect.Empty() {
				t.Fatalf("mouse region missing for %s", label)
			}
			next, cmd = m.Update(click(rect.X, rect.Y))
			got = next.(Model)
			if cmd != nil || got.ActiveView != ViewWorkspaces || got.Scope.Level != scope {
				t.Fatalf("mouse route view=%s scope=%s cmd=%v", got.ActiveView, got.Scope.Level, cmd != nil)
			}
		})
	}

	agents := NewModel(context.Background(), service, &fakeMigration{}, ViewAgents, ActionNone)
	projects := NewModel(context.Background(), service, &fakeMigration{}, ViewProjects, ActionNone)
	if agents.Scope.Level != "workspace-agents" || projects.Scope.Level != "workspace-projects" {
		t.Fatalf("aliases agents=%s projects=%s", agents.Scope.Level, projects.Scope.Level)
	}
}

func TestNavigationGroupHeadingsAreVisibleAndNotClickable(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 24
	view := stripANSI(m.ViewString())
	for _, heading := range []string{"Workspaces", "Tools"} {
		if !strings.Contains(view, heading) {
			t.Fatalf("heading %q missing:\n%s", heading, view)
		}
	}
	for _, item := range m.hitRegions().Navigation {
		if item.Entry.Label == "Workspaces" || item.Entry.Label == "Tools" {
			t.Fatalf("heading unexpectedly clickable: %+v", item)
		}
	}
}

func TestWorkspaceLandingNormalizesToProjects(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewWorkspaces, ActionNone)
	if m.Scope.Level != "workspace-projects" {
		t.Fatalf("legacy workspace scope=%q", m.Scope.Level)
	}
	for _, current := range m.rows() {
		if current.ID == "global" || current.ID == "agents" || current.ID == "projects" {
			t.Fatalf("legacy landing row remains: %+v", current)
		}
	}
}

func TestNumericNavigationMatchesMainEntries(t *testing.T) {
	want := []View{ViewOverview, ViewLibrary, ViewPresets, ViewStatus}
	for index, view := range want {
		got, ok := viewKey(string(rune('1' + index)))
		if !ok || got != view {
			t.Fatalf("key %d view=%s ok=%v want=%s", index+1, got, ok, view)
		}
	}
}

func TestCommandPaletteKeyboardRoutesWithoutMutation(t *testing.T) {
	service := &fakeService{}
	m := NewModel(context.Background(), service, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot = testSnapshot()

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = next.(Model)
	if cmd != nil || m.Mode != ModeCommand {
		t.Fatalf("ctrl+p mode=%s cmd=%v", m.Mode, cmd != nil)
	}
	for _, r := range "add source" {
		next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
		if cmd != nil {
			t.Fatalf("typing palette returned command")
		}
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil || m.Mode != ModeInput || m.Input.Kind != inputAddSource {
		t.Fatalf("add source route mode=%s kind=%s cmd=%v", m.Mode, m.Input.Kind, cmd != nil)
	}
	if service.previewAddCalls != 0 {
		t.Fatalf("palette performed add preview/mutation calls=%d", service.previewAddCalls)
	}
}

func TestColonPaletteAndEscapeRestoreCurrentPage(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewPresets, ActionNone)
	m.Snapshot = testSnapshot()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	m = next.(Model)
	if m.Mode != ModeCommand {
		t.Fatalf(": mode=%s", m.Mode)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cmd != nil || m.Mode != ModeTable || m.ActiveView != ViewPresets {
		t.Fatalf("escape mode=%s view=%s cmd=%v", m.Mode, m.ActiveView, cmd != nil)
	}
}

func TestToolsAndCommandPaletteMouseUseSameRoutes(t *testing.T) {
	service := &fakeService{}
	m := NewModel(context.Background(), service, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 24
	regions := m.hitRegions()
	var migration, configuration Rect
	for _, item := range regions.Navigation {
		switch item.Entry.Label {
		case "Migration":
			migration = item.Rect
		case "Configuration":
			configuration = item.Rect
		}
	}
	if migration.Empty() || configuration.Empty() {
		t.Fatalf("tool regions missing: migration=%+v config=%+v", migration, configuration)
	}
	next, cmd := m.Update(click(migration.X, migration.Y))
	got := next.(Model)
	if cmd != nil || got.ActiveView != ViewMigration {
		t.Fatalf("migration mouse view=%s cmd=%v", got.ActiveView, cmd != nil)
	}
	next, cmd = m.Update(click(configuration.X, configuration.Y))
	got = next.(Model)
	if cmd == nil || got.Mode != ModeConfiguration {
		t.Fatalf("configuration mouse mode=%s cmd=%v", got.Mode, cmd != nil)
	}

	m.enterCommandPalette()
	m.CommandDraft = "create preset"
	regions = m.hitRegions()
	if len(regions.Commands) != 1 {
		t.Fatalf("command regions=%d", len(regions.Commands))
	}
	next, cmd = m.Update(click(regions.Commands[0].X, regions.Commands[0].Y))
	got = next.(Model)
	if cmd != nil || got.Mode != ModeInput || got.Input.Kind != inputPresetCreate {
		t.Fatalf("palette mouse mode=%s kind=%s cmd=%v", got.Mode, got.Input.Kind, cmd != nil)
	}
}

func entryLabels(entries []navigationEntry) []string {
	result := make([]string, len(entries))
	for i := range entries {
		result[i] = entries[i].Label
	}
	return result
}
