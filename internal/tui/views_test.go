package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRenderFullAndNarrowLayoutsWithStableFooterAndErrors(t *testing.T) {
	m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	m = next.(Model)
	m.Err = "fetch failed"
	full := m.ViewString()
	for _, wanted := range []string{"Overview", "Library", "Workspaces", "Presets", "Migration", "Status", "alpha", "fetch failed", "/ Search", "? Help"} {
		if !strings.Contains(full, wanted) {
			t.Fatalf("full render missing %q:\n%s", wanted, full)
		}
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 38, Height: 12})
	m = next.(Model)
	narrow := m.ViewString()
	if !strings.Contains(narrow, "Library") || !strings.Contains(narrow, "fetch failed") || !strings.Contains(narrow, "Ctrl+Q Quit") {
		t.Fatalf("narrow render lost navigation/footer/error:\n%s", narrow)
	}
}

func TestFilterAndHelpOverlaysRender(t *testing.T) {
	m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
	m, _ = apply(m, "/")
	m, _ = apply(m, "a")
	if got := m.ViewString(); !strings.Contains(got, "Search: a") {
		t.Fatalf("filter overlay missing:\n%s", got)
	}
	m, _ = apply(m, "esc")
	m, _ = apply(m, "?")
	if got := m.ViewString(); !strings.Contains(got, "Keyboard help") || !strings.Contains(got, "space") {
		t.Fatalf("help overlay missing:\n%s", got)
	}
}
