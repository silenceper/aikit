package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/pkg/config"
)

func TestPaneFocusOrderWideForwardAndReverse(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 24
	want := []Focus{FocusNavigation, FocusList, FocusCollectionActions, FocusDetail, FocusDetailActions}
	if got := m.visibleFocusOrder(); !reflect.DeepEqual(got, want) {
		t.Fatalf("visible focus order=%v want=%v", got, want)
	}
}

func TestTabAndShiftTabVisitEveryVisiblePaneButton(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 24
	m.Focus = FocusList

	forward := []struct {
		focus Focus
		index int
	}{
		{FocusCollectionActions, 0},
		{FocusCollectionActions, 1},
		{FocusDetail, 0},
		{FocusDetailActions, 0},
		{FocusDetailActions, 1},
		{FocusNavigation, 0},
	}
	for _, want := range forward {
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(Model)
		if cmd != nil || m.Focus != want.focus || m.ActionIndex != want.index {
			t.Fatalf("Tab focus/index=%s/%d want=%s/%d cmd=%v", m.Focus, m.ActionIndex, want.focus, want.index, cmd != nil)
		}
	}

	reverse := []struct {
		focus Focus
		index int
	}{
		{FocusDetailActions, 1},
		{FocusDetailActions, 0},
		{FocusDetail, 0},
		{FocusCollectionActions, 1},
		{FocusCollectionActions, 0},
		{FocusList, 0},
	}
	for _, want := range reverse {
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
		m = next.(Model)
		if cmd != nil || m.Focus != want.focus || m.ActionIndex != want.index {
			t.Fatalf("Shift+Tab focus/index=%s/%d want=%s/%d cmd=%v", m.Focus, m.ActionIndex, want.focus, want.index, cmd != nil)
		}
	}
}

func TestNavigationKeyboardFocusActivatesExactWorkspaceRoute(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 24
	m.Focus = FocusNavigation
	entries := layoutNavigationEntries(ComputeLayout(m.Width, m.Height), m)
	for index, item := range entries {
		if item.Entry.Label == "Projects" {
			m.NavigationIndex = index
			break
		}
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if cmd != nil || got.ActiveView != ViewWorkspaces || got.Scope.Level != "workspace-projects" || got.Focus != FocusList {
		t.Fatalf("navigation route view=%s scope=%s focus=%s cmd=%v", got.ActiveView, got.Scope.Level, got.Focus, cmd != nil)
	}
}

func TestNavigationKeyboardCursorIsVisiblyDistinctFromActiveRoute(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 24
	m.Focus = FocusNavigation
	m.NavigationIndex = 1 // Library while Overview remains active.
	view := stripANSI(m.ViewString())
	if !strings.Contains(view, "> 1 Overview") || !strings.Contains(view, "{2 Library") {
		t.Fatalf("active route and navigation cursor are not independently visible:\n%s", view)
	}
}

func TestCompactFocusUsesOnlyActivePane(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 80, 20
	m.Focus = FocusList
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.Focus != FocusCollectionActions {
		t.Fatalf("collection Tab focus=%s", m.Focus)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.Focus != FocusCollectionActions || m.ActionIndex != 1 {
		t.Fatalf("collection second Tab focus/index=%s/%d", m.Focus, m.ActionIndex)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.Focus != FocusNavigation {
		t.Fatalf("collection third Tab focus=%s", m.Focus)
	}

	m.Detail, m.Focus = true, FocusDetail
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.Focus != FocusDetailActions {
		t.Fatalf("detail Tab focus=%s", m.Focus)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.Focus != FocusDetailActions || m.ActionIndex != 1 {
		t.Fatalf("detail second Tab focus/index=%s/%d", m.Focus, m.ActionIndex)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.Focus != FocusNavigation {
		t.Fatalf("detail third Tab focus=%s", m.Focus)
	}
}

func TestCompactRowCanOpenDetailWithKeyboardAndMouse(t *testing.T) {
	base := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewWorkspaces, ActionNone)
	base.Snapshot, base.Width, base.Height = testSnapshot(), 38, 18
	base.Scope = Scope{Level: "workspace-agents"}

	next, cmd := base.Update(tea.KeyMsg{Type: tea.KeyRight})
	keyboard := next.(Model)
	if cmd != nil || !keyboard.Detail || keyboard.Focus != FocusDetail {
		t.Fatalf("Right detail=%v focus=%s cmd=%v", keyboard.Detail, keyboard.Focus, cmd != nil)
	}
	if !strings.Contains(stripANSI(keyboard.ViewString()), "[Apply preset]") {
		t.Fatalf("keyboard detail actions unavailable:\n%s", keyboard.ViewString())
	}

	regions := base.hitRegions()
	if len(regions.Rows) == 0 {
		t.Fatal("compact rows missing")
	}
	row := regions.Rows[0]
	next, cmd = base.Update(click(row.X+1, row.Y))
	mouse := next.(Model)
	if cmd != nil || !mouse.Detail || mouse.Focus != FocusDetail {
		t.Fatalf("row click detail=%v focus=%s cmd=%v", mouse.Detail, mouse.Focus, cmd != nil)
	}
}

func TestWorkspaceRouteRestoresIndependentCursorAndScroll(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewWorkspaces, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 24
	m.Snapshot.Config.Projects = append(m.Snapshot.Config.Projects, config.Project{Name: "second", Path: "/work/second"})
	m.Scope, m.Cursor, m.Scroll = Scope{Level: "workspace-projects"}, 1, 1

	m.switchDestination(navigationEntry{Kind: navigationView, View: ViewWorkspaces, Scope: Scope{Level: "workspace-global"}})
	m.Cursor, m.Scroll = 1, 1
	m.switchDestination(navigationEntry{Kind: navigationView, View: ViewWorkspaces, Scope: Scope{Level: "workspace-projects"}})
	if m.Cursor != 1 || m.Scroll != 1 {
		t.Fatalf("projects position cursor=%d scroll=%d", m.Cursor, m.Scroll)
	}
}
