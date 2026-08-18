package tui

import (
	"context"
	"reflect"
	"testing"
)

func TestOverviewDashboardNavigationHidesAdvancedViewsFromRail(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 24

	wantTop := []View{ViewOverview, ViewLibrary, ViewPresets}
	if !reflect.DeepEqual(topViews, wantTop) {
		t.Fatalf("topViews=%v want=%v", topViews, wantTop)
	}
	wantLabels := []string{"Overview", "Library", "Presets", "Global", "Agents", "Projects", "Configuration", "Add source", "Create project", "Create preset"}
	if got := entryLabels(navigationEntries(m)); !reflect.DeepEqual(got, wantLabels) {
		t.Fatalf("rail entries=%v want=%v", got, wantLabels)
	}
	for _, item := range m.hitRegions().Navigation {
		if item.Entry.View == ViewStatus || item.Entry.View == ViewMigration {
			t.Fatalf("advanced view has permanent hit region: %+v", item.Entry)
		}
	}
}

func TestRetainedAdvancedRoutesStayInCommandPalette(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot = testSnapshot()

	want := map[string]View{
		"Review health details":      ViewStatus,
		"Review local skill imports": ViewMigration,
	}
	found := make(map[string]bool)
	for _, entry := range m.commandEntries() {
		view, ok := want[entry.Label]
		if !ok {
			continue
		}
		found[entry.Label] = true
		next, cmd := m.activateCommandEntry(entry)
		got := next.(Model)
		if cmd != nil || got.ActiveView != view {
			t.Fatalf("%q route view=%s cmd=%v want=%s", entry.Label, got.ActiveView, cmd != nil, view)
		}
	}
	for label := range want {
		if !found[label] {
			t.Errorf("command route %q missing", label)
		}
	}
}
