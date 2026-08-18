# Pane-owned Actions and Workspace Navigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Split list-wide and selected-item actions into their owning panes, and replace the Workspaces landing page with direct Global, Agents, and Projects navigation routes.

**Architecture:** Keep ViewWorkspaces as the internal projection owner, but add scope-aware navigation destinations so the UI opens the correct route without a landing collection. Introduce a pane-action model that independently derives Collection and Detail actions, renders and hit-tests both bars from shared geometry, and extends focus navigation without changing typed app requests or mutation safety.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, existing internal/tui layout/action primitives, table-driven Go tests.

---

### Task 1: Replace the Workspaces landing page with grouped direct routes

**Files:**
- Modify: internal/tui/navigation.go
- Modify: internal/tui/navigation_layout.go
- Modify: internal/tui/render.go
- Modify: internal/tui/model.go
- Modify: internal/tui/actions.go
- Modify: internal/tui/input_keyboard.go
- Modify: internal/tui/rows.go
- Test: internal/tui/navigation_test.go
- Test: internal/tui/usability_audit_test.go

- [ ] **Step 1: Write failing navigation tests**

Assert this exact interactive order and scope:

~~~go
want := []struct {
    label, section, scope string
}{
    {"Overview", "Main", ""},
    {"Library", "Main", ""},
    {"Presets", "Main", ""},
    {"Status", "Main", ""},
    {"Global", "Workspaces", "workspace-global"},
    {"Agents", "Workspaces", "workspace-agents"},
    {"Projects", "Workspaces", "workspace-projects"},
    {"Migration", "Tools", ""},
    {"Configuration", "Tools", ""},
}
~~~

Also assert that WORKSPACES and TOOLS are non-clickable headings; mouse, keyboard focus, command palette, ViewAgents, and ViewProjects open exact direct routes; numeric Main shortcuts match rendered labels; and the old three-row landing page cannot render.

- [ ] **Step 2: Run tests and verify RED**

Run:

~~~bash
go test ./internal/tui -run 'Test(PermanentNavigation|WorkspaceDirect|NavigationGroup|LaunchAlias|NumericNavigation)' -count=1
~~~

Expected: FAIL because navigation still exposes one Workspaces entry and activation cannot carry a workspace scope.

- [ ] **Step 3: Implement scope-aware navigation**

Extend navigationEntry with Scope and route all activation through one helper:

~~~go
type navigationEntry struct {
    Key, Label, Section string
    Kind navigationKind
    View View
    Scope Scope
    Action uiAction
}

func (m *Model) switchDestination(entry navigationEntry) {
    m.switchView(entry.View)
    m.Scope = entry.Scope
}
~~~

Use Section Workspaces for the three direct destinations. Derive rendered section headings from Section instead of hard-coding Tools; headings receive no hit region. Remove ViewWorkspaces from permanent Main destinations and remove the fallback landing rows from workspaceRows. Normalize an invalid empty workspace scope to Projects. Root Esc on Global, Agents, or Projects is a no-op; nested routes return to their direct root.

- [ ] **Step 4: Verify GREEN**

~~~bash
go test ./internal/tui -run 'Test(PermanentNavigation|WorkspaceDirect|NavigationGroup|LaunchAlias|NumericNavigation)' -count=1
go test ./internal/tui -count=1
~~~

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add internal/tui
git commit -m "feat(tui): expose direct workspace routes"
~~~

### Task 2: Define independent Collection and Detail action ownership

**Files:**
- Create: internal/tui/pane_actions.go
- Modify: internal/tui/render.go
- Modify: internal/tui/modal.go
- Modify: internal/tui/input_mouse.go
- Test: internal/tui/pane_actions_test.go
- Test: internal/tui/domain_workspace_test.go
- Test: internal/tui/workflows_test.go

- [ ] **Step 1: Write failing ownership tests**

Assert exact action ownership for Library, Global, Agents, Projects, Presets, Status, and Migration. Representative expectations:

~~~go
libraryCollection := []string{"Add source", "More"}
libraryDetail := []string{"Open", "More"}
projectCollection := []string{"Create project"}
projectDetail := []string{"Add skill", "Apply preset", "More"}
presetCollection := []string{"Create preset"}
presetDetail := []string{"Edit members", "Apply", "More"}
statusCollection := []string{"Refresh"}
~~~

Empty Library, Projects, and Presets keep Collection create actions and expose no Detail actions.

- [ ] **Step 2: Run tests and verify RED**

~~~bash
go test ./internal/tui -run TestPaneActionOwnership -count=1
~~~

Expected: FAIL because only mixed primaryActions exists.

- [ ] **Step 3: Implement pane action sets**

Create:

~~~go
type actionPane int
const (
    actionPaneCollection actionPane = iota
    actionPaneDetail
)

type paneActionSet struct {
    Collection []string
    Detail []string
}

func (m Model) paneActions() paneActionSet
func (m Model) collectionActions() []string
func (m Model) detailActions() []string
~~~

Move page action derivation out of render.go. Keep overlay actions separate. Store whether More was opened from Collection or Detail: Collection More contains filters, refresh, selection, and batch operations; Detail More contains only the active item edit/change-ref/compare/remove operations. currentActions remains the only keyboard/mouse dispatch registry and resolves the currently focused pane.

- [ ] **Step 4: Verify GREEN**

~~~bash
go test ./internal/tui -run 'Test(PaneActionOwnership|DomainWorkspace|PrimaryAction|Workflow)' -count=1
go test ./internal/tui -count=1
~~~

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add internal/tui
git commit -m "feat(tui): separate collection and detail actions"
~~~

### Task 3: Render and hit-test two independent action bars

**Files:**
- Modify: internal/tui/render.go
- Modify: internal/tui/input_mouse.go
- Modify: internal/tui/action_bar.go
- Test: internal/tui/action_bar_layout_test.go
- Test: internal/tui/input_mouse_test.go
- Test: internal/tui/render_test.go

- [ ] **Step 1: Write failing geometry tests**

At width 120 assert that Collection and Detail both render their actions; Add source is only inside CollectionPanel.Actions; Open is only inside DetailPanel.Actions; each click dispatches its own semantic request; separators and the other bar are no-ops; and paging affects only the hovered bar.

At widths 80, 59, and 24 assert that only the active page action set is visible and reachable, including empty-state create actions.

- [ ] **Step 2: Run tests and verify RED**

~~~bash
go test ./internal/tui -run 'TestDualActionBars|TestPaneActionMouseGeometry|TestCompactPaneActions' -count=1
~~~

Expected: FAIL because rendering and HitRegions choose one action area and prefer Detail.

- [ ] **Step 3: Implement shared dual-bar geometry**

Add pane-specific hit geometry:

~~~go
type PaneActionRegions struct {
    Bar, Previous, Next Rect
    Buttons []Rect
    Indexes []int
    PreviousIndex, NextIndex int
}
~~~

Use one helper that accepts an action pane, panel action rectangle, focus state, and index, then calls layoutActionBar for both rendering and hit testing. On wide layouts render both non-empty action sets. On compact/narrow render Collection actions unless a full Detail page is open. Overlay geometry remains authoritative.

- [ ] **Step 4: Verify GREEN**

~~~bash
go test ./internal/tui -run 'TestDualActionBars|TestPaneActionMouseGeometry|TestCompactPaneActions' -count=1
go test ./internal/tui -count=1
~~~

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add internal/tui
git commit -m "feat(tui): render pane-owned action bars"
~~~

### Task 4: Add navigation and pane-aware focus

**Files:**
- Modify: internal/tui/model.go
- Modify: internal/tui/input_keyboard.go
- Modify: internal/tui/input_mouse.go
- Modify: internal/tui/modal.go
- Test: internal/tui/action_focus_test.go
- Test: internal/tui/navigation_test.go
- Test: internal/tui/foundation_test.go

- [ ] **Step 1: Write failing focus tests**

Assert the wide cycle Navigation -> Collection -> Collection actions -> Detail -> Detail actions, forward and reverse Tab, Up/Down plus Enter in Navigation, left/right within each action bar, root and nested Escape behavior, and mouse/keyboard parity. Cursor, scroll, filters, and selection must not change during focus traversal.

At compact/narrow widths, Collection pages skip Detail focus and full Detail pages skip Collection action focus. Global, Agents, and Projects preserve independent cursor/scroll state.

- [ ] **Step 2: Run tests and verify RED**

~~~bash
go test ./internal/tui -run 'TestPaneFocusOrder|TestNavigationKeyboardFocus|TestWorkspaceRouteState' -count=1
~~~

Expected: FAIL because there is one generic FocusActions, no Navigation focus, and no route position state.

- [ ] **Step 3: Implement explicit focus targets**

~~~go
const (
    FocusNavigation Focus = "navigation"
    FocusList Focus = "list"
    FocusCollectionActions Focus = "collection-actions"
    FocusDetail Focus = "detail"
    FocusDetailActions Focus = "detail-actions"
    FocusActions Focus = "overlay-actions"
)

type routePosition struct {
    Cursor, Scroll int
    ActiveKey string
}
~~~

Preserve FocusActions for modals so modal reset guarantees remain intact. Store NavigationIndex and workspace route positions. Build traversal from currently visible targets rather than nested special cases. Navigation skips section headings, and Enter calls activateCommandEntry, the same dispatcher used by mouse and Ctrl+P.

- [ ] **Step 4: Verify GREEN**

~~~bash
go test ./internal/tui -run 'TestPaneFocusOrder|TestNavigationKeyboardFocus|TestWorkspaceRouteState' -count=1
go test ./internal/tui -count=1
~~~

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add internal/tui
git commit -m "feat(tui): add pane-aware focus navigation"
~~~

### Task 5: Verify responsive operability and the real TUI

**Files:**
- Modify only if a regression exposes a defect: internal/tui/**
- Test: internal/tui/operability_regression_test.go
- Test: internal/tui/usability_audit_test.go

- [ ] **Step 1: Add end-to-end regression tests**

Cover keyboard and mouse paths for Library collection/detail actions; direct Global, Agents, and Projects navigation; Project Create versus item Add skill/Apply preset; Preset Create versus item Edit members/Apply; Status Refresh versus issue actions; cancellation issuing zero mutations; and recovery/busy gates blocking both bars.

- [ ] **Step 2: Run full automated verification**

~~~bash
go test ./internal/tui -count=1
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
GOOS=windows GOARCH=amd64 go build ./...
git diff --check
~~~

Expected: every command exits 0.

- [ ] **Step 3: Run end-to-end and real-data smoke tests**

~~~bash
make test-e2e
make run
~~~

Using the real local configuration, resize wide/compact/narrow and verify no overflow; both wide bars remain in their owning frames; workspace routes open directly; every visible action is keyboard/mouse reachable; empty states retain create actions; Esc backs out without quitting; Ctrl+Q exits. Open previews only and cancel without confirming mutations.

- [ ] **Step 4: Review the final diff against the specification**

~~~bash
git status --short
git diff --stat
git diff -- internal/tui
~~~

Confirm every specification verification item and that no typed app service, mutation, or recovery contract changed.

- [ ] **Step 5: Commit**

~~~bash
git add internal/tui
git commit -m "test(tui): verify pane-owned workspace navigation"
~~~
