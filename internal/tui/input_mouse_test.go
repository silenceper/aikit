package tui

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/pkg/config"
)

func TestModernMouseGeometryUsesNavigationAndTwoLineRows(t *testing.T) {
	for _, width := range []int{120, 80, 59, 38} {
		t.Run(fmt.Sprintf("width-%d", width), func(t *testing.T) {
			m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
			m.Snapshot, m.Width, m.Height = testSnapshot(), width, 20
			regions := m.hitRegions()
			for _, item := range layoutNavigation(regions.Layout, topViews, m.ActiveView) {
				if got := regions.Tabs[item.View]; !reflect.DeepEqual(got, item.Rect) {
					t.Fatalf("navigation %s rect=%+v want=%+v", item.View, got, item.Rect)
				}
			}
			rowHeight := collectionRowHeight
			if !regions.Layout.Detail.Empty() {
				rowHeight = 1
			}
			if len(regions.Rows) < 2 || regions.Rows[0].Height != rowHeight || regions.Rows[1].Y != regions.Rows[0].Y+rowHeight {
				t.Fatalf("row rects=%+v", regions.Rows)
			}
			clickY := regions.Rows[1].Y
			if rowHeight > 1 {
				clickY++
			}
			next, cmd := m.Update(click(regions.Rows[1].X+1, clickY))
			got := next.(Model)
			if cmd != nil || got.Cursor != 1 {
				t.Fatalf("second physical row line selected cursor=%d cmd=%v", got.Cursor, cmd != nil)
			}
		})
	}
}

func TestLibraryRowMouseClickTogglesSelectionAcrossLayouts(t *testing.T) {
	for _, width := range []int{120, 80, 59, 38} {
		t.Run(fmt.Sprintf("width-%d", width), func(t *testing.T) {
			base := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
			base.Snapshot, base.Width, base.Height = testSnapshot(), width, 20
			regions := base.hitRegions()
			if len(regions.Rows) < 2 || len(regions.Checkboxes) < 2 {
				t.Fatalf("missing Library row geometry: rows=%+v checkboxes=%+v", regions.Rows, regions.Checkboxes)
			}

			row := regions.Rows[1]
			clickX := row.Right() - 1
			clickY := row.Y
			if row.Height > 1 {
				clickY = row.Bottom() - 1
			}
			next, cmd := base.Update(click(clickX, clickY))
			selected := next.(Model)
			key := selected.rows()[1].selectionKey()
			if cmd != nil || selected.Cursor != 1 || !selected.Selected[key] {
				t.Fatalf("row click did not select exact skill: cursor=%d selected=%v cmd=%v", selected.Cursor, selected.Selected, cmd != nil)
			}

			regions = selected.hitRegions()
			row = regions.Rows[1]
			clickY = row.Y
			if row.Height > 1 {
				clickY = row.Bottom() - 1
			}
			next, cmd = selected.Update(click(row.Right()-1, clickY))
			got := next.(Model)
			if ComputeLayout(width, selected.Height).Detail.Empty() {
				if cmd != nil || !got.Detail || !got.Selected[key] {
					t.Fatalf("second compact row click did not open selected skill detail: detail=%v selected=%v cmd=%v", got.Detail, got.Selected, cmd != nil)
				}
				next, _ = got.Update(actionKey(tea.KeyEsc))
				got = next.(Model)
				checkbox := got.hitRegions().Checkboxes[1]
				next, cmd = got.Update(click(checkbox.X, checkbox.Y))
				got = next.(Model)
			}
			if cmd != nil || got.Selected[key] {
				t.Fatalf("second wide row click/compact checkbox did not deselect exact skill: selected=%v cmd=%v", got.Selected, cmd != nil)
			}
		})
	}
}

func TestCompactOverlayKeepsTitleAndPrimaryActionsVisible(t *testing.T) {
	for _, height := range []int{8, 12} {
		t.Run(fmt.Sprintf("height-%d", height), func(t *testing.T) {
			m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
			m.Snapshot, m.Width, m.Height = testSnapshot(), 80, height
			m.Mode = ModeConfiguration
			m.Config = app.ConfigurationDetail{Config: "/config", Library: "/library", Cache: "/cache"}
			m.ConfigValidation = app.ConfigurationValidation{Path: "/config", Valid: true}
			got := stripANSI(m.ViewString())
			if !strings.Contains(got, "Configuration") || !strings.Contains(got, "[Validate]") {
				t.Fatalf("short overlay lost fixed title/action:\n%s", got)
			}
			regions := m.hitRegions()
			if len(regions.Actions) == 0 || !regions.Layout.Overlay.Contains(regions.Actions[0].X, regions.Actions[0].Y) {
				t.Fatalf("short overlay action outside panel: overlay=%+v actions=%+v", regions.Layout.Overlay, regions.Actions)
			}
		})
	}
}

func TestOverviewAttentionRenderAndHitShareGeometry(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 80, 28
	m.Inventory.Items = []app.ScanItem{
		{Key: "error", State: app.ScanStateError, Skill: config.Skill{Name: "error item"}},
		{Key: "drift", State: app.ScanStateDrifted, Skill: config.Skill{Name: "drift item"}},
	}
	m.OverviewSection = overviewHealth
	regions := m.hitRegions()
	healthRows := regions.OverviewRows[overviewHealth]
	wantY := m.overviewLayout(regions.Layout).Health.Body.Y
	if len(healthRows) == 0 || healthRows[0].Y != wantY || healthRows[0].Height != collectionRowHeight {
		t.Fatalf("overview row geometry=%+v want first y=%d height=%d", healthRows, wantY, collectionRowHeight)
	}
	plain := strings.Split(stripANSI(m.ViewString()), "\n")
	if !strings.Contains(plain[healthRows[0].Y], m.overviewDashboard().Health[0].Name) {
		t.Fatalf("hit row does not align with rendered row at y=%d:\n%s", healthRows[0].Y, m.ViewString())
	}
	next, cmd := m.Update(click(healthRows[1].X, healthRows[1].Y))
	got := next.(Model)
	if cmd != nil || got.Cursor != 1 || got.OverviewSection != overviewHealth {
		t.Fatalf("overview second-line click cursor=%d cmd=%v", got.Cursor, cmd != nil)
	}
}

func TestOverviewKeyboardScrollUsesSharedRowCapacity(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 80, 12
	m.Snapshot.Status.Items = nil
	for i := 0; i < 12; i++ {
		m.Inventory.Items = append(m.Inventory.Items, app.ScanItem{Key: fmt.Sprintf("item-%02d", i), State: app.ScanStateUnmanaged, Skill: config.Skill{Name: fmt.Sprintf("item-%02d", i)}})
	}
	for i := 0; i < 11; i++ {
		next, _ := m.perform(uiMoveDown)
		m = next.(Model)
	}
	geometry := m.visibleRowsLayout(ComputeLayout(m.Width, m.Height))
	if m.Scroll != geometry.Start || m.Scroll == 0 {
		t.Fatalf("overview scroll=%d shared start=%d cursor=%d", m.Scroll, geometry.Start, m.Cursor)
	}
}

func TestNarrowBreadcrumbMouseBackUsesParentNavigation(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewWorkspaces, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 38, 16
	m.Scope = Scope{Project: "aikit", Agent: "codex", Level: "project-skills"}
	regions := m.hitRegions()
	if regions.Back.Empty() || !strings.Contains(stripANSI(strings.Split(m.ViewString(), "\n")[regions.Back.Y]), "‹ ") {
		t.Fatalf("narrow breadcrumb Back not rendered/hittable: %+v\n%s", regions.Back, m.ViewString())
	}
	next, cmd := m.Update(click(regions.Back.X, regions.Back.Y))
	got := next.(Model)
	if cmd != nil || got.ActiveView != ViewWorkspaces || got.Scope.Level != "project-targets" || got.Scope.Project != "aikit" {
		t.Fatalf("Back used view switch instead of uiBack: view=%s scope=%+v cmd=%v", got.ActiveView, got.Scope, cmd != nil)
	}
}

func TestConfirmAndInputOverlayButtonsSharePanelGeometry(t *testing.T) {
	for _, height := range []int{8, 12} {
		t.Run(fmt.Sprintf("confirm-%d", height), func(t *testing.T) {
			m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
			m.Snapshot, m.Width, m.Height = testSnapshot(), 80, height
			m.Mode, m.Preview.Summary = ModeConfirm, "Review exact action"
			regions := m.hitRegions()
			if regions.Confirm.Empty() || regions.Cancel.Empty() || !regions.Layout.Overlay.Contains(regions.Confirm.X, regions.Confirm.Y) || !regions.Layout.Overlay.Contains(regions.Cancel.X, regions.Cancel.Y) {
				t.Fatalf("confirm buttons not inside panel: overlay=%+v confirm=%+v cancel=%+v", regions.Layout.Overlay, regions.Confirm, regions.Cancel)
			}
			line := strings.Split(stripANSI(m.ViewString()), "\n")[regions.Confirm.Y]
			if !strings.Contains(line, "[Confirm]") || !strings.Contains(line, "[Cancel]") {
				t.Fatalf("confirm render/hit mismatch at y=%d: %q\n%s", regions.Confirm.Y, line, m.ViewString())
			}
		})

		t.Run(fmt.Sprintf("input-%d", height), func(t *testing.T) {
			service := &fakeService{}
			m := NewModel(context.Background(), service, &fakeMigration{}, ViewLibrary, ActionNone)
			m.Snapshot, m.Width, m.Height = testSnapshot(), 80, height
			m.Mode, m.Input = ModeInput, inputState{Kind: inputAddSource, Prompt: "Source", Value: "/tmp/example"}
			regions := m.hitRegions()
			if regions.Confirm.Empty() || regions.Cancel.Empty() || !regions.Layout.Overlay.Contains(regions.Confirm.X, regions.Confirm.Y) {
				t.Fatalf("input buttons not inside panel: overlay=%+v apply=%+v cancel=%+v", regions.Layout.Overlay, regions.Confirm, regions.Cancel)
			}
			line := strings.Split(stripANSI(m.ViewString()), "\n")[regions.Confirm.Y]
			if !strings.Contains(line, "[Apply]") || !strings.Contains(line, "[Cancel]") {
				t.Fatalf("input render/hit mismatch at y=%d: %q\n%s", regions.Confirm.Y, line, m.ViewString())
			}
			next, cmd := m.Update(click(regions.Confirm.X, regions.Confirm.Y))
			got := next.(Model)
			if cmd == nil || !got.Busy || service.previewAddCalls != 0 {
				t.Fatalf("mouse Apply did not use input submit: cmd=%v busy=%v calls=%d", cmd != nil, got.Busy, service.previewAddCalls)
			}
		})
	}
}

func TestConfirmPanelBodyScrollReachesFinalItem(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 80, 12
	m.Mode = ModeConfirm
	for i := 0; i < 10; i++ {
		m.Preview.Warnings = append(m.Preview.Warnings, fmt.Sprintf("body-%02d", i))
	}
	for i := 0; i < 20; i++ {
		m.moveOverlayScroll(1)
	}
	if !strings.Contains(m.ViewString(), "body-09") {
		t.Fatalf("confirm body cannot reach final item at scroll=%d:\n%s", m.OverlayScroll, m.ViewString())
	}
}

func TestCompactLibraryDetailDoesNotCreateAnUnboundedScrollRegion(t *testing.T) {
	base := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	base.Snapshot, base.Width, base.Height = testSnapshot(), 80, 12
	base.Detail, base.Focus, base.Cursor, base.Scroll = true, FocusDetail, 1, 1
	base.SkillDetail.Skill = base.Snapshot.Config.Library.Skills[1]
	for i := 0; i < 12; i++ {
		base.SkillDetail.Files = append(base.SkillDetail.Files, app.SkillFile{Path: fmt.Sprintf("file-%02d", i), Kind: app.SkillFileRegular})
	}

	keyboardNext, cmd := base.Update(actionKey(tea.KeyDown))
	keyboard := keyboardNext.(Model)
	if cmd != nil || keyboard.DetailScroll != 0 || keyboard.Cursor != base.Cursor || keyboard.Scroll != base.Scroll {
		t.Fatalf("detail key scroll=%d cursor=%d listScroll=%d cmd=%v", keyboard.DetailScroll, keyboard.Cursor, keyboard.Scroll, cmd != nil)
	}

	layout := ComputeLayout(base.Width, base.Height)
	mouseNext, cmd := base.Update(tea.MouseMsg{X: layout.Main.X, Y: layout.Main.Y + 1, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	mouse := mouseNext.(Model)
	if cmd != nil || mouse.DetailScroll != keyboard.DetailScroll || mouse.Cursor != base.Cursor || mouse.Scroll != base.Scroll || mouse.Focus != FocusDetail {
		t.Fatalf("detail mouse parity scroll=%d cursor=%d listScroll=%d focus=%s cmd=%v", mouse.DetailScroll, mouse.Cursor, mouse.Scroll, mouse.Focus, cmd != nil)
	}
	if mouse.ViewString() != base.ViewString() {
		t.Fatal("compact detail unexpectedly exposed another scroll page")
	}
}

func click(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

func TestMouseTabsRowsCheckboxWheelAndKeyboardParity(t *testing.T) {
	base := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	base.Snapshot = testSnapshot()
	base.Inventory = InventoryState{Items: []app.ScanItem{
		{Key: "one", Skill: config.Skill{Name: "one"}, State: app.ScanStateUnmanaged, Action: app.ScanActionAdopt},
		{Key: "two", Skill: config.Skill{Name: "two"}, State: app.ScanStateManaged},
	}}
	base.Width, base.Height = 100, 20

	regions := base.hitRegions()
	var projects Rect
	for _, item := range regions.Navigation {
		if item.Entry.Label == "Projects" {
			projects = item.Rect
		}
	}
	mouseNext, _ := base.Update(click(projects.X, projects.Y))
	keyboard := base
	keyboard.enterCommandPalette()
	keyboard.CommandDraft = "projects"
	keyboardNext, _ := keyboard.Update(key("enter"))
	if mouseNext.(Model).ActiveView != keyboardNext.(Model).ActiveView || mouseNext.(Model).Scope.Level != "workspace-projects" {
		t.Fatalf("tab parity mouse=%s keyboard=%s", mouseNext.(Model).ActiveView, keyboardNext.(Model).ActiveView)
	}

	m := base
	m.switchView(ViewMigration)
	regions = m.hitRegions()
	next, _ := m.Update(click(regions.Rows[1].X+2, regions.Rows[1].Y))
	m = next.(Model)
	if m.Cursor != 1 {
		t.Fatalf("row click cursor=%d", m.Cursor)
	}
	next, _ = m.Update(click(regions.Checkboxes[0].X, regions.Checkboxes[0].Y))
	m = next.(Model)
	if !m.Selected["one"] {
		t.Fatalf("checkbox click did not select exact key: %v", m.Selected)
	}

	wheel := tea.MouseMsg{X: regions.Layout.List.X, Y: regions.Layout.List.Y, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown}
	next, _ = m.Update(wheel)
	if next.(Model).Cursor <= m.Cursor {
		t.Fatalf("wheel did not use list movement: before=%d after=%d", m.Cursor, next.(Model).Cursor)
	}
}

func TestMouseConfirmCancelBusyGate(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewMigration, ActionNone)
	m.Width, m.Height = 80, 20
	m.Inventory = InventoryState{Items: []app.ScanItem{{Key: "one", Origin: "g/codex", Target: "/tmp/one", Skill: config.Skill{Name: "one"}, State: app.ScanStateUnmanaged, Action: app.ScanActionAdopt}}}
	m.Selected["one"] = true
	m.Mode, m.confirm = ModeConfirm, ActionScan

	regions := m.hitRegions()
	next, cmd := m.Update(click(regions.Cancel.X, regions.Cancel.Y))
	if cmd != nil || next.(Model).Mode == ModeConfirm {
		t.Fatalf("mouse cancel failed: mode=%s cmd=%v", next.(Model).Mode, cmd != nil)
	}

	m.Busy = true
	regions = m.hitRegions()
	next, cmd = m.Update(click(regions.Confirm.X, regions.Confirm.Y))
	if cmd != nil || !next.(Model).Busy {
		t.Fatal("busy model accepted repeated confirm click")
	}
}
