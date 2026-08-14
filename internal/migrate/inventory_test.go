package migrate

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func TestInventoryStreamEmitsEachAuthorizedRootInDeterministicOrder(t *testing.T) {
	service, paths, home := testService(t)
	for _, item := range agent.All() {
		writeSkill(t, filepath.Join(item.GlobalSkillDir(home), "global-"+item.Name()), "global-"+item.Name(), item.Name())
	}
	alpha, zeta := filepath.Join(t.TempDir(), "alpha"), filepath.Join(t.TempDir(), "zeta")
	cfg := config.New()
	cfg.Projects = []config.Project{
		{Name: "zeta", Path: zeta, Agents: []string{"windsurf", "cursor"}, AgentBindings: map[string]config.Binding{}},
		{Name: "alpha", Path: alpha, Agents: []string{"codex", "claude-code"}, AgentBindings: map[string]config.Binding{}},
	}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	for _, project := range cfg.Projects {
		for _, name := range project.Agents {
			item, _ := agent.ByName(name)
			writeSkill(t, filepath.Join(item.ProjectSkillDir(project.Path), project.Name+"-"+name), project.Name+"-"+name, name)
		}
	}
	beforeConfig, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	beforeLibrary := snapshotTree(t, paths.LibrarySkills)
	service.deps.Recover = func(string, []config.PendingOperation, link.Selector, bool) link.Result {
		t.Fatal("inventory called recovery")
		return link.Result{}
	}
	service.deps.BeforeMutationValidation = func() { t.Fatal("inventory entered mutation validation") }

	events := collectInventoryEvents(t, service.Inventory(context.Background(), app.InventoryRequest{Generation: 73, AllProjects: true}))
	wantRoots := []string{
		"g/cursor", "g/claude-code", "g/codex", "g/copilot", "g/windsurf",
		"p/alpha/claude-code", "p/alpha/codex", "p/zeta/cursor", "p/zeta/windsurf",
	}
	if len(events) != len(wantRoots) {
		t.Fatalf("event count = %d, want %d: %+v", len(events), len(wantRoots), events)
	}
	for index, event := range events {
		if event.Generation != 73 || event.Root != wantRoots[index] {
			t.Fatalf("event %d identity = generation %d root %q, want 73/%q", index, event.Generation, event.Root, wantRoots[index])
		}
		if event.Completed != index+1 || event.Total != len(wantRoots) || event.Done != (index == len(wantRoots)-1) {
			t.Fatalf("event %d progress = completed %d total %d done %v", index, event.Completed, event.Total, event.Done)
		}
		for itemIndex, item := range event.Items {
			if item.Origin != event.Root {
				t.Fatalf("event %q item origin = %q", event.Root, item.Origin)
			}
			if itemIndex > 0 && event.Items[itemIndex-1].Target > item.Target {
				t.Fatalf("event %q items are not stably sorted: %+v", event.Root, event.Items)
			}
		}
	}
	afterConfig, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeConfig, afterConfig) {
		t.Fatal("inventory changed config bytes")
	}
	if after := snapshotTree(t, paths.LibrarySkills); !reflect.DeepEqual(beforeLibrary, after) {
		t.Fatalf("inventory changed library tree: before=%#v after=%#v", beforeLibrary, after)
	}
	if _, err := os.Stat(paths.Lock); !os.IsNotExist(err) {
		t.Fatalf("inventory acquired persistent lock: %v", err)
	}

	again := collectInventoryEvents(t, service.Inventory(context.Background(), app.InventoryRequest{Generation: 74, AllProjects: true}))
	gotRoots := make([]string, len(again))
	for index := range again {
		gotRoots[index] = again[index].Root
	}
	if !reflect.DeepEqual(gotRoots, wantRoots) {
		t.Fatalf("second root order = %#v, want %#v", gotRoots, wantRoots)
	}
}

func TestInventoryStreamReportsRootErrorsWithoutStoppingOtherRoots(t *testing.T) {
	service, _, home := testService(t)
	out := t.TempDir()
	cursorRoot := agent.Cursor{}.GlobalSkillDir(home)
	if err := os.MkdirAll(filepath.Dir(cursorRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, cursorRoot); err != nil {
		t.Fatal(err)
	}
	codexTarget := filepath.Join(agent.Codex{}.GlobalSkillDir(home), "healthy")
	writeSkill(t, codexTarget, "healthy", "content")

	events := collectInventoryEvents(t, service.Inventory(context.Background(), app.InventoryRequest{Generation: 8}))
	if len(events) != len(agent.All()) {
		t.Fatalf("event count = %d, want %d", len(events), len(agent.All()))
	}
	byRoot := make(map[string]app.InventoryEvent, len(events))
	for _, event := range events {
		byRoot[event.Root] = event
	}
	if issues := byRoot["g/cursor"].Issues; len(issues) != 1 || issues[0].Origin != "g/cursor" || issues[0].State != app.ScanStateError {
		t.Fatalf("cursor issues = %+v", issues)
	}
	if items := byRoot["g/codex"].Items; len(items) != 1 || items[0].Target != codexTarget {
		t.Fatalf("healthy root items = %+v", items)
	}
	if !events[len(events)-1].Done {
		t.Fatal("completed partial inventory did not report Done")
	}
}

func TestInventoryStreamCancellationClosesWithoutCompletionOrWorkerLeak(t *testing.T) {
	service, _, home := testService(t)
	target := filepath.Join(agent.Cursor{}.GlobalSkillDir(home), "blocked")
	writeSkill(t, target, "blocked", "content")
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	cancelObserved := make(chan struct{})
	allowInspectReturn := make(chan struct{})
	inspectReturned := make(chan struct{})
	service.deps.InventoryInspect = func(ctx context.Context, path, libraryRoot string) (link.State, error) {
		if path != target {
			return link.Inspect(path, libraryRoot)
		}
		close(started)
		<-ctx.Done()
		close(cancelObserved)
		<-allowInspectReturn
		close(inspectReturned)
		return link.State{}, ctx.Err()
	}

	events := make(chan app.InventoryEvent)
	inventoryReturned := make(chan struct{})
	go func() {
		service.inventory(ctx, app.InventoryRequest{Generation: 99}, events)
		close(inventoryReturned)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("inventory worker did not reach blocked root")
	}
	cancel()
	select {
	case <-cancelObserved:
	case <-time.After(time.Second):
		t.Fatal("context-aware inventory inspect did not observe cancellation")
	}
	select {
	case <-inventoryReturned:
		t.Fatal("inventory returned while a canceled worker was still running")
	case <-time.After(50 * time.Millisecond):
	}
	close(allowInspectReturn)
	select {
	case <-inspectReturned:
	case <-time.After(time.Second):
		t.Fatal("context-aware inventory inspect did not return after cancellation")
	}
	select {
	case <-inventoryReturned:
	case <-time.After(time.Second):
		t.Fatal("inventory function did not return after all workers and the waiter completed")
	}
	for event := range events {
		if event.Done {
			t.Fatalf("canceled generation emitted completion: %+v", event)
		}
	}
}

func TestInventoryStreamCanceledBeforeStartDoesNoWork(t *testing.T) {
	service, _, _ := testService(t)
	var inspected atomic.Bool
	service.deps.InventoryInspect = func(context.Context, string, string) (link.State, error) {
		inspected.Store(true)
		return link.State{}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	events := collectInventoryEvents(t, service.Inventory(ctx, app.InventoryRequest{Generation: 101, AllProjects: true}))
	if len(events) != 0 {
		t.Fatalf("pre-canceled inventory events = %+v", events)
	}
	if inspected.Load() {
		t.Fatal("pre-canceled inventory invoked discovery")
	}
}

func collectInventoryEvents(t *testing.T, events <-chan app.InventoryEvent) []app.InventoryEvent {
	t.Helper()
	done := make(chan []app.InventoryEvent, 1)
	go func() {
		var result []app.InventoryEvent
		for event := range events {
			result = append(result, event)
		}
		done <- result
	}()
	select {
	case result := <-done:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("inventory stream did not close")
		return nil
	}
}
