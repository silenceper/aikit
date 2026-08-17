package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/pkg/config"
)

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestNarrowDetailUsesFullWidthAndEscReturnsToList(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot = testSnapshot()
	m.Width, m.Height = 38, 12

	m, _ = apply(m, "enter")
	if !m.Detail {
		t.Fatal("Enter did not open detail")
	}
	rendered := m.ViewString()
	if !strings.Contains(rendered, "aikit / Library / alpha") || !strings.Contains(rendered, "State: Update available") || strings.Contains(rendered, "Library skills") || !strings.Contains(rendered, "Esc Back") {
		t.Fatalf("narrow detail did not replace list full-width:\n%s", rendered)
	}
	m, cmd := apply(m, "esc")
	if cmd != nil || m.Detail || !strings.Contains(m.ViewString(), "Library skills") {
		t.Fatalf("Esc did not return to list: detail=%v cmd=%v\n%s", m.Detail, cmd != nil, m.ViewString())
	}
}

func TestOverlayMouseCapturesUnderlyingTabsRowsAndActions(t *testing.T) {
	base := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	base.Snapshot = testSnapshot()
	base.Width, base.Height = 100, 20
	underlying := base.hitRegions()

	tests := []struct {
		name  string
		setup func(*Model)
	}{
		{"help", func(m *Model) { m.Help = true }},
		{"configuration", func(m *Model) { m.Mode = ModeConfiguration }},
		{"filter", func(m *Model) { m.Mode, m.filterParent = ModeFilter, ModeTable }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := base
			tt.setup(&m)
			for _, point := range []Rect{underlying.Tabs[ViewMigration], underlying.Rows[0], underlying.Actions[0]} {
				next, cmd := m.Update(click(point.X, point.Y))
				got := next.(Model)
				if cmd != nil || got.ActiveView != ViewLibrary || got.Cursor != m.Cursor || got.MutationBusy {
					t.Fatalf("overlay leaked click at %+v: view=%s cursor=%d mutationBusy=%v cmd=%v", point, got.ActiveView, got.Cursor, got.MutationBusy, cmd != nil)
				}
			}
		})
	}
}

func TestCheckboxRegionsExistOnlyWhenSelectionIsRendered(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewWorkspaces, ActionNone)
	m.Snapshot = testSnapshot()
	m.Scope = Scope{Agent: "codex", Level: "agent-skills"}
	m.Width, m.Height = 100, 20
	regions := m.hitRegions()
	if len(regions.Checkboxes) != 0 || strings.Contains(stripANSI(m.ViewString()), "[ ]") {
		t.Fatalf("workspace rendered selectable checkbox: regions=%d\n%s", len(regions.Checkboxes), m.ViewString())
	}
	next, cmd := m.Update(click(regions.Rows[0].X, regions.Rows[0].Y))
	got := next.(Model)
	if cmd != nil || got.MutationBusy || got.Cursor != 0 {
		t.Fatalf("workspace row click triggered mutation: busy=%v cmd=%v cursor=%d", got.MutationBusy, cmd != nil, got.Cursor)
	}

	m.switchView(ViewMigration)
	m.Inventory.Items = testInventoryItems()
	regions = m.hitRegions()
	if len(regions.Checkboxes) != len(regions.Rows) || !strings.Contains(stripANSI(m.ViewString()), "[ ]") {
		t.Fatalf("migration checkbox render/hit mismatch: checkboxes=%d rows=%d\n%s", len(regions.Checkboxes), len(regions.Rows), m.ViewString())
	}
}

func TestMutationBusyRejectsNormalQuitAllowsEmergencyAndStillHandlesResult(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot = testSnapshot()
	m.MutationBusy = true
	m.Busy = true
	for _, input := range []string{"ctrl+q", "q"} {
		next, cmd := apply(m, input)
		if isQuit(cmd) || !next.MutationBusy || !strings.Contains(strings.ToLower(next.Status), "operation in progress") {
			t.Fatalf("mutation busy accepted %q: quit=%v mutationBusy=%v status=%q", input, isQuit(cmd), next.MutationBusy, next.Status)
		}
	}
	if _, cmd := apply(m, "ctrl+c"); !isQuit(cmd) {
		t.Fatal("mutation busy rejected emergency ctrl+c")
	}

	next, refresh := m.Update(operationMsg{name: "update"})
	got := next.(Model)
	if got.MutationBusy || got.Busy || refresh == nil || !strings.Contains(got.Status, "Updated") {
		t.Fatalf("mutation result not handled: mutationBusy=%v busy=%v refresh=%v status=%q", got.MutationBusy, got.Busy, refresh != nil, got.Status)
	}
}

func testInventoryItems() []app.ScanItem {
	return []app.ScanItem{{Key: "one", Skill: config.Skill{Name: "one"}, State: app.ScanStateUnmanaged, Action: app.ScanActionAdopt}}
}
