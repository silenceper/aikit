package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestLibraryWorkspaceUsesOneLineRowsAndBoundedDetail(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 20
	m.Snapshot.Config.Library.Skills[0].Description = strings.Repeat("说明👩‍💻", 100)
	regions := m.hitRegions()
	if len(regions.Rows) < 2 || regions.Rows[0].Height != 1 || regions.Rows[1].Y != regions.Rows[0].Y+1 {
		t.Fatalf("library rows are not one-line: %+v", regions.Rows)
	}
	lines := strings.Split(m.ViewString(), "\n")
	for index, line := range lines {
		if lipgloss.Width(line) > m.Width {
			t.Fatalf("line %d width=%d > %d: %q", index, lipgloss.Width(line), m.Width, line)
		}
	}
	if !strings.Contains(stripANSI(m.ViewString()), "Summary") || !strings.Contains(stripANSI(m.ViewString()), "Usage") {
		t.Fatalf("live detail missing summary/usage:\n%s", m.ViewString())
	}
}

func TestLibraryWholeRowClickTogglesExactlyOnceInFramedLayout(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 20
	regions := m.hitRegions()
	row := regions.Rows[0]
	next, cmd := m.Update(click(row.Right()-1, row.Y))
	got := next.(Model)
	key := got.rows()[0].selectionKey()
	if cmd != nil || !got.Selected[key] {
		t.Fatalf("whole row did not select: selected=%v cmd=%v", got.Selected, cmd != nil)
	}
	next, cmd = got.Update(click(row.Right()-1, row.Y))
	got = next.(Model)
	if cmd != nil || got.Selected[key] {
		t.Fatalf("second click did not deselect exactly once: selected=%v", got.Selected)
	}
}
