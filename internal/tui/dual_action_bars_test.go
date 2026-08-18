package tui

import (
	"context"
	"strings"
	"testing"
)

func TestDualActionBarsRenderInOwningPanels(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 24
	regions := m.hitRegions()
	if regions.CollectionActions.Bar.Empty() || regions.DetailActions.Bar.Empty() {
		t.Fatalf("dual bars missing collection=%+v detail=%+v", regions.CollectionActions.Bar, regions.DetailActions.Bar)
	}
	if !regions.Layout.CollectionPanel.Actions.Contains(regions.CollectionActions.Bar.X, regions.CollectionActions.Bar.Y) {
		t.Fatalf("collection bar outside owner: panel=%+v bar=%+v", regions.Layout.CollectionPanel.Actions, regions.CollectionActions.Bar)
	}
	if !regions.Layout.DetailPanel.Actions.Contains(regions.DetailActions.Bar.X, regions.DetailActions.Bar.Y) {
		t.Fatalf("detail bar outside owner: panel=%+v bar=%+v", regions.Layout.DetailPanel.Actions, regions.DetailActions.Bar)
	}
	plain := stripANSI(m.ViewString())
	for _, label := range []string{"[Add source]", "[Open]"} {
		if !strings.Contains(plain, label) {
			t.Fatalf("render missing %q:\n%s", label, plain)
		}
	}
}

func TestPaneActionMouseGeometryDispatchesOwningAction(t *testing.T) {
	base := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	base.Snapshot, base.Width, base.Height = testSnapshot(), 120, 24

	addRect := paneButtonRect(t, base.collectionActions(), base.hitRegions().CollectionActions, "Add source")
	next, cmd := base.Update(click(addRect.X, addRect.Y))
	added := next.(Model)
	if cmd != nil || added.Mode != ModeInput || added.Input.Kind != inputAddSource {
		t.Fatalf("collection click mode=%s kind=%s cmd=%v", added.Mode, added.Input.Kind, cmd != nil)
	}

	openRect := paneButtonRect(t, base.detailActions(), base.hitRegions().DetailActions, "Open")
	next, cmd = base.Update(click(openRect.X, openRect.Y))
	opened := next.(Model)
	if cmd == nil || !opened.Detail || opened.pendingDetailID != "acme/alpha" {
		t.Fatalf("detail click detail=%v id=%q cmd=%v", opened.Detail, opened.pendingDetailID, cmd != nil)
	}
}

func TestCompactPaneActionsSwitchWithActivePage(t *testing.T) {
	for _, width := range []int{80, 59, 24} {
		t.Run(string(rune(width)), func(t *testing.T) {
			m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
			m.Snapshot, m.Width, m.Height = testSnapshot(), width, 18
			regions := m.hitRegions()
			if regions.CollectionActions.Bar.Empty() || !regions.DetailActions.Bar.Empty() {
				t.Fatalf("width=%d collection=%+v detail=%+v", width, regions.CollectionActions.Bar, regions.DetailActions.Bar)
			}
			if !strings.Contains(stripANSI(m.ViewString()), "[Add source]") {
				t.Fatalf("width=%d collection action missing:\n%s", width, m.ViewString())
			}

			m.Detail = true
			regions = m.hitRegions()
			if !regions.CollectionActions.Bar.Empty() || regions.DetailActions.Bar.Empty() {
				t.Fatalf("detail width=%d collection=%+v detail=%+v", width, regions.CollectionActions.Bar, regions.DetailActions.Bar)
			}
			plain := stripANSI(m.ViewString())
			if !strings.Contains(plain, "[Open]") || strings.Contains(plain, "[Add source]") {
				t.Fatalf("detail width=%d wrong actions:\n%s", width, plain)
			}
		})
	}
}

func paneButtonRect(t *testing.T, actions []string, regions PaneActionRegions, label string) Rect {
	t.Helper()
	for visible, index := range regions.Indexes {
		if index < len(actions) && actions[index] == label && visible < len(regions.Buttons) {
			return regions.Buttons[visible]
		}
	}
	t.Fatalf("button %q missing actions=%v regions=%+v", label, actions, regions)
	return Rect{}
}
