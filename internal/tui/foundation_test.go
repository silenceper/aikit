package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

type inventoryCall struct {
	ctx     context.Context
	request app.InventoryRequest
	ch      chan app.InventoryEvent
}

type startupMigration struct {
	app.MigrationService
	calls []inventoryCall
}

func (f *startupMigration) Inventory(ctx context.Context, request app.InventoryRequest) <-chan app.InventoryEvent {
	call := inventoryCall{ctx: ctx, request: request, ch: make(chan app.InventoryEvent, 4)}
	f.calls = append(f.calls, call)
	return call.ch
}

func TestStartupSnapshotIsOfflineAndInventoryIsIncremental(t *testing.T) {
	service := &fakeService{snapshot: testSnapshot()}
	migration := &startupMigration{}
	m := NewModel(context.Background(), service, migration, ViewOverview, ActionNone)

	msg := m.Init()()
	if !service.lastSnapshot.Offline || service.snapshotCalls != 1 || len(migration.calls) != 0 {
		t.Fatalf("startup before snapshot: request=%+v snapshotCalls=%d inventoryCalls=%d", service.lastSnapshot, service.snapshotCalls, len(migration.calls))
	}
	next, wait := m.Update(msg)
	m = next.(Model)
	if wait == nil || len(migration.calls) != 1 || !migration.calls[0].request.AllProjects || migration.calls[0].request.Generation == 0 {
		t.Fatalf("inventory startup = %+v calls=%d wait=%v", migration.calls, len(migration.calls), wait != nil)
	}
	if !strings.Contains(m.ViewString(), "Overview") || strings.Contains(m.ViewString(), "loose-one") {
		t.Fatalf("snapshot did not render before inventory:\n%s", m.ViewString())
	}

	generation := migration.calls[0].request.Generation
	migration.calls[0].ch <- app.InventoryEvent{Generation: generation, Root: "g/codex", Items: []app.ScanItem{{Key: "one", Origin: "g/codex", Target: "/tmp/one", Skill: config.Skill{ID: "local/one", Name: "loose-one"}, State: app.ScanStateUnmanaged, Action: app.ScanActionAdopt}}, Completed: 1, Total: 2}
	next, wait = m.Update(wait())
	m = next.(Model)
	if len(m.Inventory.Items) != 1 || !m.Inventory.Loading || wait == nil {
		t.Fatalf("first inventory merge = %+v wait=%v", m.Inventory, wait != nil)
	}
	migration.calls[0].ch <- app.InventoryEvent{Generation: generation, Root: "p/aikit/codex", Issues: []app.ScanIssue{{State: app.ScanStateError, Origin: "p/aikit/codex", Message: "permission denied"}}, Completed: 2, Total: 2, Done: true}
	next, _ = m.Update(wait())
	m = next.(Model)
	if m.Inventory.Loading || !m.Inventory.Complete || len(m.Inventory.Issues) != 1 || m.Inventory.Issues[0].Message != "permission denied" {
		t.Fatalf("completed inventory = %+v\n%s", m.Inventory, m.ViewString())
	}
}

func TestStartupAutomaticallyChecksUpdatesOnceUsingCache(t *testing.T) {
	service := &fakeService{
		snapshot: testSnapshot(),
		updateResult: app.Result{Updates: updatecheck.CheckReport{Results: []updatecheck.Result{{
			SkillID: "acme/alpha", Current: strings.Repeat("a", 40), Remote: strings.Repeat("c", 40), State: updatecheck.StateUpdateAvailable,
		}}}},
	}
	migration := &startupMigration{}
	m := NewModel(context.Background(), service, migration, ViewOverview, ActionNone)

	next, wait := m.Update(m.Init()())
	m = next.(Model)
	generation := migration.calls[0].request.Generation
	migration.calls[0].ch <- app.InventoryEvent{Generation: generation, Completed: 1, Total: 1, Done: true}
	next, check := m.Update(wait())
	m = next.(Model)
	if check == nil || !m.Busy || m.Activity.Kind != ActivityNetwork || service.updateCalls != 0 {
		t.Fatalf("automatic check did not start after inventory: busy=%v activity=%v calls=%d cmd=%v", m.Busy, m.Activity.Kind, service.updateCalls, check != nil)
	}

	message, ok := check().(activityResultMsg)
	if !ok {
		t.Fatalf("automatic check command returned %T", check())
	}
	next, _ = m.Update(message)
	m = next.(Model)
	if service.updateCalls != 1 || !service.lastUpdate.CheckOnly || service.lastUpdate.Refresh || service.lastUpdate.Confirmed {
		t.Fatalf("automatic update request=%+v calls=%d", service.lastUpdate, service.updateCalls)
	}
	if len(m.Snapshot.Updates.Results) != 1 || m.Snapshot.Updates.Results[0].State != updatecheck.StateUpdateAvailable {
		t.Fatalf("automatic update result=%+v", m.Snapshot.Updates)
	}

	next, follow := m.Update(inventoryMsg{event: app.InventoryEvent{Generation: generation, Completed: 1, Total: 1, Done: true}, ok: true})
	m = next.(Model)
	if follow != nil {
		_ = follow()
	}
	if service.updateCalls != 1 {
		t.Fatalf("automatic update check repeated: calls=%d", service.updateCalls)
	}
}

func TestStartupRefreshCancelsGenerationAndIgnoresStaleEvents(t *testing.T) {
	migration := &startupMigration{}
	m := NewModel(context.Background(), &fakeService{snapshot: testSnapshot()}, migration, ViewMigration, ActionNone)
	next, _ := m.Update(m.Init()())
	m = next.(Model)
	old := migration.calls[0]
	m.Inventory.Items = []app.ScanItem{{Key: "keep", Skill: config.Skill{Name: "keep"}}}
	m.Cursor = 0
	m.Selected["keep"] = true

	m, _ = apply(m, "r")
	if len(migration.calls) != 2 || migration.calls[1].request.Generation == old.request.Generation {
		t.Fatalf("refresh calls=%+v", migration.calls)
	}
	select {
	case <-old.ctx.Done():
	default:
		t.Fatal("refresh did not cancel old inventory context")
	}

	next, cmd := m.Update(inventoryMsg{event: app.InventoryEvent{Generation: old.request.Generation, Items: []app.ScanItem{{Key: "stale", Skill: config.Skill{Name: "stale"}}}}, ok: true})
	m = next.(Model)
	if cmd != nil || len(m.Inventory.Items) != 0 || !m.Selected["keep"] {
		t.Fatalf("stale event changed refreshed model: %+v selected=%v", m.Inventory, m.Selected)
	}
}

func TestStartupIncrementalMergePreservesIdentitySelectionAndScroll(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &startupMigration{}, ViewMigration, ActionNone)
	m.Inventory = InventoryState{Generation: 9, Loading: true, Items: []app.ScanItem{
		{Key: "a", Skill: config.Skill{Name: "a"}}, {Key: "b", Skill: config.Skill{Name: "b"}}, {Key: "c", Skill: config.Skill{Name: "c"}},
	}}
	m.Cursor, m.Scroll = 1, 1
	m.Selected["c"] = true
	next, _ := m.Update(inventoryMsg{event: app.InventoryEvent{Generation: 9, Items: []app.ScanItem{{Key: "a", Skill: config.Skill{Name: "a2"}}, {Key: "d", Skill: config.Skill{Name: "d"}}}}, ok: true})
	m = next.(Model)
	if got := m.rows()[m.Cursor].selectionKey(); got != "b" || m.Scroll != 1 || !m.Selected["c"] {
		t.Fatalf("identity changed: cursorKey=%q scroll=%d selected=%v items=%+v", got, m.Scroll, m.Selected, m.Inventory.Items)
	}
}

func TestOverviewIncrementalLocalMergePreservesSelectedViewportAnchor(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &startupMigration{}, ViewOverview, ActionNone)
	m.OverviewSection = overviewLocal
	m.Snapshot = testSnapshot()
	m.Snapshot.Status.Items = nil
	m.Width, m.Height = 80, 16
	m.Inventory = InventoryState{Generation: 7, Loading: true}
	for i := 0; i < 8; i++ {
		m.Inventory.Items = append(m.Inventory.Items, app.ScanItem{Key: fmt.Sprintf("item-%02d", i), State: app.ScanStateUnmanaged, Skill: config.Skill{Name: fmt.Sprintf("item-%02d", i)}})
	}
	m.Cursor, m.Scroll = 4, 2
	selectedKey := m.rows()[m.Cursor].selectionKey()
	selectedOffset := m.Cursor - m.Scroll

	next, _ := m.Update(inventoryMsg{event: app.InventoryEvent{Generation: 7, Items: []app.ScanItem{{Key: "new-error", State: app.ScanStateError, Skill: config.Skill{Name: "new error"}}}}, ok: true})
	m = next.(Model)
	if got := m.rows()[m.Cursor].selectionKey(); got != selectedKey {
		t.Fatalf("selected key changed: got=%q want=%q", got, selectedKey)
	}
	if got := m.Cursor - m.Scroll; got != selectedOffset {
		t.Fatalf("viewport anchor changed: cursor=%d scroll=%d offset=%d want=%d", m.Cursor, m.Scroll, got, selectedOffset)
	}
}
