package tui

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
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

func TestOverviewSectionsAlwaysExposeQuickActions(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	dashboard := m.overviewDashboard()
	if got := dashboard.sectionIDs(); !reflect.DeepEqual(got, []overviewSectionID{overviewQuick, overviewUpdates, overviewLocal, overviewHealth}) {
		t.Fatalf("sections=%v", got)
	}
	if got := dashboard.QuickActions; !reflect.DeepEqual(got, []uiAction{uiAddSource, uiCreateProject, uiCreatePreset}) {
		t.Fatalf("quick actions=%v", got)
	}
}

func TestOverviewUpdateTasksRequireCompleteTypedTokens(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot = testSnapshot()
	m.Snapshot.Config.Library.Skills[0].Ref = &config.Ref{Kind: "branch", Value: "main"}
	m.Snapshot.Updates.Results = append(m.Snapshot.Updates.Results,
		updatecheck.Result{SkillID: "acme/beta", Current: strings.Repeat("b", 40), State: updatecheck.StateCheckFailed, Error: "offline"},
	)
	dashboard := m.overviewDashboard()
	if len(dashboard.Updates) != 2 {
		t.Fatalf("updates=%+v", dashboard.Updates)
	}
	if task := dashboard.Updates[0]; !task.Selectable || task.SkillID != "acme/alpha" || task.Current == "" || task.Remote == "" || task.SelectionKey != "overview:update:acme/alpha" {
		t.Fatalf("valid update task=%+v", task)
	}
	if task := dashboard.Updates[1]; task.Selectable || task.State != "Update check failed" || task.Detail != "offline" {
		t.Fatalf("failed update task=%+v", task)
	}

	m.Snapshot.Config.Library.Skills[0].Ref = nil
	if task := m.overviewDashboard().Updates[0]; task.Selectable {
		t.Fatalf("update without ref is selectable: %+v", task)
	}
}

func TestOverviewLocalTasksKeepExactSelectorsAndDisableUnsafeRows(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Inventory.Items = []app.ScanItem{
		{Key: "safe", Origin: "g/codex", Target: "/work/.codex/skills/safe", ContentHash: "hash", ObjectID: "object", RootObjectID: "root", State: app.ScanStateUnmanaged, Action: app.ScanActionAdopt, Skill: config.Skill{ID: "local/safe", Name: "safe"}},
		{Key: "conflict", Origin: "g/claude", Target: "/work/.claude/skills/safe", State: app.ScanStateNameConflict, Action: app.ScanActionConflict, Skill: config.Skill{ID: "local/other", Name: "safe"}},
	}
	tasks := m.overviewDashboard().Local
	if len(tasks) != 2 {
		t.Fatalf("local tasks=%+v", tasks)
	}
	if !tasks[0].Selectable || tasks[0].Selector.Key != "safe" || tasks[0].Selector.Origin != "g/codex" || tasks[0].Selector.Target != "/work/.codex/skills/safe" || tasks[0].Selector.ExpectedObjectID != "object" {
		t.Fatalf("safe local task=%+v", tasks[0])
	}
	if tasks[1].Selectable || tasks[1].Action != app.ScanActionConflict {
		t.Fatalf("unsafe local task=%+v", tasks[1])
	}
}

func TestOverviewHealthTasksKeepExactDestinations(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot = testSnapshot()
	m.Snapshot.Config.PendingOperations = []config.PendingOperation{{ID: "recover-1"}}
	tasks := m.overviewDashboard().Health
	if len(tasks) < 2 || tasks[0].DestinationAction != ActionRecovery {
		t.Fatalf("health tasks=%+v", tasks)
	}
	foundStatus := false
	for _, task := range tasks {
		if task.DestinationView == ViewStatus && strings.HasPrefix(task.DestinationKey, "status:") {
			foundStatus = true
		}
	}
	if !foundStatus {
		t.Fatalf("exact status destination missing: %+v", tasks)
	}
}

func TestOverviewDashboardRenderShowsFourTaskSections(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 28
	m.Inventory.Items = []app.ScanItem{{Key: "safe", Origin: "g/codex", Target: "/work/.codex/skills/safe", State: app.ScanStateUnmanaged, Action: app.ScanActionAdopt, Skill: config.Skill{ID: "local/safe", Name: "safe"}}}
	plain := stripANSI(m.ViewString())
	for _, want := range []string{"Task dashboard", "Quick actions", "Add skill", "Add project", "Create preset", "Updates", "Local skills", "Needs attention", "alpha", "safe"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("dashboard missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "Workspace summary") {
		t.Fatalf("legacy Overview panel remains:\n%s", plain)
	}
	for _, line := range strings.Split(plain, "\n") {
		if len([]rune(line)) > m.Width {
			t.Fatalf("line exceeds width %d: %q", m.Width, line)
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

func TestOverviewSectionsKeepIndependentCursorAndScroll(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 80, 16
	m.Snapshot.Status.Items = nil
	for i := 0; i < 10; i++ {
		m.Inventory.Items = append(m.Inventory.Items, app.ScanItem{
			Key: fmt.Sprintf("item-%02d", i), State: app.ScanStateUnmanaged,
			Action: app.ScanActionAdopt, Skill: config.Skill{Name: fmt.Sprintf("item-%02d", i)},
		})
	}
	m.OverviewSection, m.Cursor, m.Scroll = overviewLocal, 6, 3
	wantKey := m.activeKey()

	m.switchOverviewSection(overviewHealth)
	m.Cursor, m.Scroll = 1, 1
	m.switchOverviewSection(overviewLocal)

	if got := m.activeKey(); got != wantKey || m.Cursor != 6 || m.Scroll != 3 {
		t.Fatalf("local position not restored: key=%q cursor=%d scroll=%d want key=%q cursor=6 scroll=3", got, m.Cursor, m.Scroll, wantKey)
	}
}

func TestOverviewIncrementalInventoryInvalidatesUnsafeSelection(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.OverviewSection = overviewLocal
	m.Inventory = InventoryState{Generation: 11, Loading: true, Items: []app.ScanItem{{
		Key: "candidate", Origin: "g/codex", Target: "/work/candidate", State: app.ScanStateUnmanaged,
		Action: app.ScanActionAdopt, Skill: config.Skill{ID: "local/candidate", Name: "candidate"},
	}}}
	m.Selected["overview:local:candidate"] = true

	next, _ := m.Update(inventoryMsg{ok: true, event: app.InventoryEvent{Generation: 11, Items: []app.ScanItem{{
		Key: "candidate", Origin: "g/codex", Target: "/work/candidate", State: app.ScanStateNameConflict,
		Action: app.ScanActionConflict, Skill: config.Skill{ID: "local/candidate", Name: "candidate"},
	}}}})
	m = next.(Model)
	if m.Selected["overview:local:candidate"] {
		t.Fatalf("unsafe inventory replacement remained selected: %+v", m.Selected)
	}
}
