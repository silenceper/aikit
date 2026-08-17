package tui

import (
	"context"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPermanentNavigationAndToolsAreSeparated(t *testing.T) {
	want := []View{ViewOverview, ViewLibrary, ViewWorkspaces, ViewPresets, ViewStatus}
	if !reflect.DeepEqual(topViews, want) {
		t.Fatalf("topViews=%v want=%v", topViews, want)
	}
	entries := navigationEntries(Model{})
	if got := entryLabels(entries[:7]); !reflect.DeepEqual(got, []string{"Overview", "Library", "Workspaces", "Presets", "Status", "Migration", "Configuration"}) {
		t.Fatalf("entries=%v", got)
	}
	if entries[5].Section != "Tools" || entries[6].Section != "Tools" {
		t.Fatalf("tools not separated: %+v", entries)
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
