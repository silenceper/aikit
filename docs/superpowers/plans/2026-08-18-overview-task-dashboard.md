# Overview Task Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Overview the primary action dashboard for adding skills, applying verified updates, importing local skills, and resolving health issues while removing Status and Migration from permanent navigation.

**Architecture:** Keep the existing typed app and migration workflows authoritative. Add an Overview-only projection with stable section and row identities, a dedicated responsive layout shared by rendering and mouse hit-testing, and semantic dispatchers that translate dashboard actions into the existing preview/confirm commands. Split permanent navigation from command-palette routes so Status and Migration remain reachable without occupying the rail.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, existing `internal/tui` layout/action primitives, table-driven Go tests.

---

### Task 1: Separate permanent navigation from retained advanced routes

**Files:**
- Modify: `internal/tui/navigation.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/input_keyboard.go`
- Test: `internal/tui/overview_dashboard_test.go`
- Test: `internal/tui/navigation_test.go`

- [ ] **Step 1: Write failing navigation tests**

Assert that the permanent rail contains only Overview, Library, Presets, Global, Agents, Projects, and Configuration; `topViews` and numeric shortcuts contain exactly Overview, Library, and Presets; and neither Status nor Migration receives a rail hit rectangle.

Also assert that `Ctrl+P` still contains `Review health details` and `Review local skill imports`, and activates the retained exact `ViewStatus` and `ViewMigration` routes.

- [ ] **Step 2: Run tests and verify RED**

```bash
go test ./internal/tui -run 'TestOverviewDashboardNavigation|TestRetainedAdvancedRoutes' -count=1
```

Expected: FAIL because `navigationEntries` currently drives both the rail and command palette and still exposes Status and Migration.

- [ ] **Step 3: Implement separate projections**

Keep `navigationEntries` as the permanent rail source and add `commandNavigationEntries` that appends retained advanced routes and task actions. Change `commandEntries` to use the command projection. Reduce `topViews` to the three visible Main entries and preserve hidden-view route state.

- [ ] **Step 4: Verify GREEN**

```bash
go test ./internal/tui -run 'TestOverviewDashboardNavigation|TestRetainedAdvancedRoutes|TestNumericNavigation' -count=1
go test ./internal/tui -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/tui
git commit -m "feat(tui): simplify primary navigation"
```

### Task 2: Define typed Overview sections and stable task rows

**Files:**
- Create: `internal/tui/overview.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/rows.go`
- Modify: `internal/tui/actions.go`
- Test: `internal/tui/overview_dashboard_test.go`

- [ ] **Step 1: Write failing projection tests**

Cover four stable sections: Quick actions, Updates, Local skills, and Needs attention. Assert:

- quick actions are present on an empty configuration;
- updates contain only complete `UpdateAvailable` tokens and expose current/remote revisions;
- failed checks remain diagnostics and are not selectable;
- local executable rows retain backend selector, origin, target, action, and stable key;
- conflict, drift, error, and pending recovery rows are inspectable but disabled;
- health rows keep exact Status composite keys and Recovery IDs; and
- severity sorting and incremental inventory merging preserve the active stable row.

- [ ] **Step 2: Run tests and verify RED**

```bash
go test ./internal/tui -run 'TestOverview(Sections|UpdateTasks|LocalTasks|HealthTasks|IncrementalIdentity)' -count=1
```

Expected: FAIL because Overview is a single flattened attention list.

- [ ] **Step 3: Implement the Overview projection**

Add explicit `overviewSectionID`, `overviewTask`, and `overviewDashboard` models. Derive them only from `Snapshot`, `Inventory`, and pending recovery. Prefix selection keys by section so update and local selections remain independent. Never create requests or inspect files while projecting.

Store the active section, per-section cursor, scroll, and active row key in `Model`. Reconcile invalid selections after every inventory or update result merge.

- [ ] **Step 4: Verify GREEN**

```bash
go test ./internal/tui -run 'TestOverview(Sections|UpdateTasks|LocalTasks|HealthTasks|IncrementalIdentity)' -count=1
go test ./internal/tui -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/tui
git commit -m "feat(tui): project Overview task sections"
```

### Task 3: Render the responsive task dashboard from shared geometry

**Files:**
- Create: `internal/tui/overview_layout.go`
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/input_mouse.go`
- Modify: `internal/tui/row_layout.go`
- Test: `internal/tui/overview_layout_test.go`
- Test: `internal/tui/overview_dashboard_test.go`

- [ ] **Step 1: Write failing render and hit tests**

At widths 120, 80, 59, 38, and 24 assert bounded layout and shared hit rectangles. Wide layout renders Quick actions across the top, Updates and Local skills side-by-side, and Needs attention across the bottom. Compact layout stacks sections. Narrow layout expands one active section and provides a breadcrumb/back target.

Use long paths, CJK, and emoji fixtures and assert no line exceeds the terminal display-cell width. Assert section headings, row hit boxes, selection boxes, review actions, and empty-state quick actions map to the rendered cells.

- [ ] **Step 2: Run tests and verify RED**

```bash
go test ./internal/tui -run 'TestOverview(DashboardLayout|DashboardMouseGeometry|DashboardNarrow|DashboardClipping)' -count=1
```

Expected: FAIL because Overview still uses the generic collection/detail panel and one flattened row geometry.

- [ ] **Step 3: Implement shared Overview geometry**

Add a pure `layoutOverview` primitive returning section panels, visible row rectangles, checkbox rectangles, section action rectangles, and narrow breadcrumb geometry. Render and hit-test exclusively from that result. Use existing grapheme/display-cell clipping and panel glyphs. Keep each section bounded and show counts/ranges when rows are clipped.

- [ ] **Step 4: Verify GREEN**

```bash
go test ./internal/tui -run 'TestOverview(DashboardLayout|DashboardMouseGeometry|DashboardNarrow|DashboardClipping)' -count=1
go test ./internal/tui -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/tui
git commit -m "feat(tui): render responsive Overview dashboard"
```

### Task 4: Route Quick actions and verified update batches

**Files:**
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/pane_actions.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/input_mouse.go`
- Modify: `internal/tui/commands.go`
- Test: `internal/tui/overview_workflows_test.go`

- [ ] **Step 1: Write failing workflow tests**

Assert keyboard/mouse parity for Add skill, Add project, and Create preset, reusing the existing input states and typed requests. Assert startup sends only offline Snapshot plus Inventory and performs zero update/Git calls.

For updates, assert explicit Check updates performs exactly one refresh check; valid rows can be selected independently; `Update selected` rejects missing/stale/current-mismatch tokens; preview and cancel perform zero writes; and confirmation sends one `app.Batch{Operation: BatchUpdate}` with complete per-skill `Expected` values.

- [ ] **Step 2: Run tests and verify RED**

```bash
go test ./internal/tui -run 'TestOverview(QuickActions|StartupOffline|CheckUpdates|UpdateBatch)' -count=1
```

- [ ] **Step 3: Implement semantic dispatch**

Reuse `uiAddSource`, `uiCreateProject`, `uiCreatePreset`, `updateCheckCmd`, `libraryBatchRequest`, and the existing confirm/batch handlers. Add Overview selection adapters that produce the same library selection IDs and validate every cached update token before entering confirmation. Do not add a second backend mutation path.

- [ ] **Step 4: Verify GREEN**

```bash
go test ./internal/tui -run 'TestOverview(QuickActions|StartupOffline|CheckUpdates|UpdateBatch)' -count=1
go test ./internal/tui ./internal/app ./internal/updatecheck -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/tui
git commit -m "feat(tui): add Overview update workflows"
```

### Task 5: Route exact local-skill and health workflows

**Files:**
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/input_mouse.go`
- Test: `internal/tui/overview_workflows_test.go`

- [ ] **Step 1: Write failing workflow tests**

Assert executable local rows select exact backend `ScanSelector` values and a mixed unsafe selection is rejected. Confirming produces one typed migration request with exact origin, target, key, object identity, hash, state, and action. Cancel is zero-write. `Review all` and complex rows open retained Migration at the stable key.

Assert health actions route exact Status composite keys, Sync preview, status Adopt, and Recovery preview IDs. Unknown/conflicting content never receives a direct repair action. Errors and partial typed results remain visible.

- [ ] **Step 2: Run tests and verify RED**

```bash
go test ./internal/tui -run 'TestOverview(LocalSelection|LocalPreview|HealthActions|RetainedDetailRoutes)' -count=1
```

- [ ] **Step 3: Implement exact action adapters**

Translate Overview local selections into the existing `selectedSelectors`, `selectedTargets`, migration preview, and confirmation flows without weakening their tokens. Translate health tasks into the existing `openAttention`, sync preview, status adopt, and recovery preview paths. Preserve hidden-view cursor restoration by stable destination key.

- [ ] **Step 4: Verify GREEN**

```bash
go test ./internal/tui -run 'TestOverview(LocalSelection|LocalPreview|HealthActions|RetainedDetailRoutes)' -count=1
go test ./internal/tui ./internal/migrate ./internal/app -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/tui
git commit -m "feat(tui): connect Overview local and health tasks"
```

### Task 6: Complete focus, responsiveness, and release verification

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/input_mouse.go`
- Modify: `internal/tui/render.go`
- Test: `internal/tui/overview_dashboard_test.go`
- Test: `internal/tui/overview_workflows_test.go`
- Test: `internal/tui/overview_layout_test.go`

- [ ] **Step 1: Write failing interaction regression tests**

Cover Tab focus among navigation, sections, rows, and active actions; `j/k`, arrows, Space, Enter, mouse click, wheel, and narrow breadcrumb; independent selection; selection invalidation; Busy blocking; root Escape no-exit; and Ctrl+Q-only normal exit. Assert incremental events preserve section, row identity, viewport offset, and selections.

- [ ] **Step 2: Run tests and verify RED**

```bash
go test ./internal/tui -run 'TestOverview(Focus|KeyboardMouseParity|Busy|IncrementalViewport|ExitSafety)' -count=1
```

- [ ] **Step 3: Implement the remaining interaction state**

Centralize Overview focus/selection transitions so keyboard and mouse call the same semantic helpers. Clamp each section independently after resize or data changes. Keep ordinary root Escape as a no-op and Ctrl+Q as the intentional normal exit.

- [ ] **Step 4: Run full verification**

```bash
go test ./internal/tui -count=1
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
GOOS=windows GOARCH=amd64 go build ./...
make test-e2e
git diff --check
gofmt -l internal/tui
```

Expected: every command exits 0 and `gofmt -l` prints nothing.

- [ ] **Step 5: Perform real-config read-only smoke verification**

Run `make run` against the user's real configuration. Inspect Overview at wide, compact, and narrow widths; navigate Quick actions, Updates, Local skills, and Needs attention with keyboard and mouse; open retained Status/Migration details; cancel every preview; and verify config/library files are unchanged.

- [ ] **Step 6: Commit**

```bash
git add internal/tui
git commit -m "test(tui): verify Overview task dashboard"
```
