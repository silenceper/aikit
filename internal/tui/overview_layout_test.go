package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOverviewDashboardLayoutUsesTaskSections(t *testing.T) {
	for _, width := range []int{120, 80, 59, 38, 24} {
		t.Run(string(rune(width)), func(t *testing.T) {
			m := Model{Width: width, Height: 24, ActiveView: ViewOverview, OverviewSection: overviewUpdates}
			geometry := m.overviewLayout(ComputeLayout(width, 24))
			if geometry.Quick.Outer.Empty() || geometry.ActivePanel().Outer.Empty() {
				t.Fatalf("width %d missing quick/active panels: %+v", width, geometry)
			}
			for name, panel := range map[string]PanelLayout{"quick": geometry.Quick, "updates": geometry.Updates, "local": geometry.Local, "health": geometry.Health} {
				if !panel.Outer.Empty() && (!geometry.Outer.Contains(panel.Outer.X, panel.Outer.Y) || panel.Outer.Right() > geometry.Outer.Right() || panel.Outer.Bottom() > geometry.Outer.Bottom()) {
					t.Fatalf("width %d %s outside main: panel=%+v outer=%+v", width, name, panel.Outer, geometry.Outer)
				}
			}
			if width >= 96 {
				if geometry.Updates.Outer.Empty() || geometry.Local.Outer.Empty() || geometry.Health.Outer.Empty() || geometry.Updates.Outer.Y != geometry.Local.Outer.Y || geometry.Updates.Outer.Overlaps(geometry.Local.Outer) || geometry.Health.Outer.Y <= geometry.Updates.Outer.Y {
					t.Fatalf("wide geometry=%+v", geometry)
				}
			} else if width < 60 {
				if !geometry.Local.Outer.Empty() || !geometry.Health.Outer.Empty() || geometry.SectionBar.Empty() {
					t.Fatalf("narrow must show only active task section: %+v", geometry)
				}
			}
		})
	}
}

func TestOverviewDashboardLayoutSharesVisibleRowRects(t *testing.T) {
	m := Model{Width: 120, Height: 24, ActiveView: ViewOverview, OverviewSection: overviewUpdates}
	m.Snapshot = testSnapshot()
	geometry := m.overviewLayout(ComputeLayout(m.Width, m.Height))
	if len(geometry.Rows[overviewUpdates].Rects) == 0 {
		t.Fatalf("update row rects missing: %+v", geometry.Rows)
	}
	for _, rect := range geometry.Rows[overviewUpdates].Rects {
		if !geometry.Updates.Body.Contains(rect.X, rect.Y) || rect.Right() > geometry.Updates.Body.Right() || rect.Bottom() > geometry.Updates.Body.Bottom() {
			t.Fatalf("row rect outside update body: row=%+v body=%+v", rect, geometry.Updates.Body)
		}
	}
}

func TestOverviewDashboardMouseGeometryMatchesRenderedSections(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 24
	m.Inventory.Items = testInventoryItems()
	regions := m.hitRegions()
	geometry := m.overviewLayout(regions.Layout)
	for _, section := range []overviewSectionID{overviewUpdates, overviewLocal, overviewHealth} {
		if got := regions.OverviewSections[section]; got != geometry.panel(section).Outer {
			t.Fatalf("section %s hit=%+v render=%+v", section, got, geometry.panel(section).Outer)
		}
		if len(regions.OverviewRows[section]) != len(geometry.Rows[section].Rects) {
			t.Fatalf("section %s rows=%d render=%d", section, len(regions.OverviewRows[section]), len(geometry.Rows[section].Rects))
		}
	}
	if len(regions.OverviewQuick.Buttons) != 3 {
		t.Fatalf("quick buttons=%+v", regions.OverviewQuick)
	}
}

func TestOverviewNarrowSectionMouseCyclesLikeKeyboard(t *testing.T) {
	base := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	base.Snapshot, base.Width, base.Height = testSnapshot(), 38, 20
	base.OverviewSection = overviewUpdates
	regions := base.hitRegions()
	if regions.OverviewNext.Empty() || regions.OverviewPrevious.Empty() {
		t.Fatalf("narrow section controls missing: prev=%+v next=%+v", regions.OverviewPrevious, regions.OverviewNext)
	}

	next, mouseCmd := base.Update(tea.MouseMsg{X: regions.OverviewNext.X, Y: regions.OverviewNext.Y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	mouse := next.(Model)
	keyboard := base
	next, keyboardCmd := keyboard.Update(tea.KeyMsg{Type: tea.KeyRight})
	keyboard = next.(Model)
	if mouseCmd != nil || keyboardCmd != nil || mouse.OverviewSection != overviewLocal || keyboard.OverviewSection != mouse.OverviewSection {
		t.Fatalf("section parity mouse=%s keyboard=%s", mouse.OverviewSection, keyboard.OverviewSection)
	}
}
