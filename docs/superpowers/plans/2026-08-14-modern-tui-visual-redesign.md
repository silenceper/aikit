# Modern TUI Visual Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restyle the existing English aikit TUI as a calm, modern skills manager with responsive navigation, attention-first overview, two-line collections, and exact keyboard/mouse geometry while preserving every current workflow.

**Architecture:** Introduce a semantic adaptive theme and make `ComputeLayout` the sole source of shell geometry. Rendering consumes semantic rows and layout rectangles; mouse hit testing consumes the same rectangles and action-bar primitives. Business requests, preview/confirmation flows, startup inventory, and service interfaces remain untouched.

**Tech Stack:** Go, Bubble Tea v1, Lip Gloss v1.1, table-driven Go tests.

---

## File map

- Modify `internal/tui/styles.go`: semantic palette, adaptive/reduced/no-color styles, badges, focus markers.
- Modify `internal/tui/layout.go`: wide/compact/single-pane shell geometry and visible-row capacity.
- Create `internal/tui/navigation_layout.go`: single source of per-navigation-item labels and hit rectangles.
- Modify `internal/tui/render.go`: app bar, navigation rail, metrics, two-line rows, grouped details, footer, and overlays.
- Modify `internal/tui/rows.go`: presentation metadata and deterministic attention ordering only.
- Modify `internal/tui/input_mouse.go`: hit regions derived from layout/render geometry.
- Modify `internal/tui/model.go`: presentation-only theme/focus defaults if required; no service contract changes.
- Modify/add tests under `internal/tui/*_test.go`: semantic, geometry, keyboard/mouse parity, theme fallbacks.

Because this is a shared dirty worktree with existing user changes, implementation steps do not create intermediate commits. They must use `apply_patch`, preserve unrelated changes, and finish with a scoped diff review.

### Task 1: Semantic adaptive theme

**Files:**
- Modify: `internal/tui/styles.go`
- Test: `internal/tui/styles_test.go`
- Test: `internal/tui/render_test.go`

- [ ] **Step 1: Write failing palette tests**

Add table-driven tests for dark, light, reduced-color, and no-color themes. Assert active focus and success/warning/error remain distinguishable after `stripANSI`, using markers or labels rather than color alone.

- [ ] **Step 2: Run the tests and verify RED**

Run: `go test ./internal/tui -run 'TestSemanticTheme|TestNoColorFocusAndSeverity' -count=1`

Expected: FAIL because the semantic palette and injected theme constructor do not exist.

- [ ] **Step 3: Implement the minimal semantic palette**

Create a package-local `theme` containing styles for app title, navigation, selected row, muted context, success, warning, error, badge, panel title, and primary action. Use `lipgloss.CompleteAdaptiveColor` with ANSI fallbacks; make no-color/reduced behavior explicit and testable. Retain `clip`, `clipPlain`, and `stripANSI` behavior.

- [ ] **Step 4: Run focused tests**

Run: `go test ./internal/tui -run 'TestSemanticTheme|TestNoColorFocusAndSeverity' -count=1`

Expected: PASS.

### Task 2: Responsive shell geometry

**Files:**
- Modify: `internal/tui/layout.go`
- Create: `internal/tui/navigation_layout.go`
- Modify: `internal/tui/layout_test.go`

- [ ] **Step 1: Write failing boundary tests**

Test widths `120, 96, 95, 80, 60, 59, 38, 24` and heights `8, 12, 16, 30`. Require:

- wide (`>=96`): full navigation rail plus one main workspace;
- compact (`60-95`): short rail plus one main workspace;
- narrow (`<60`): breadcrumb single pane;
- overlay, status, and footer inside bounds;
- title and actions remain visible at heights 8 and 12;
- no non-empty rectangles overlap unexpectedly.

- [ ] **Step 2: Run the layout tests and verify RED**

Run: `go test ./internal/tui -run 'TestModernShell|TestLayoutResponsiveRectsStayInsideTerminal' -count=1`

Expected: FAIL because the current layout uses horizontal tabs and the old width-55 split.

- [ ] **Step 3: Implement the responsive shell**

Extend `Layout` with navigation and main-workspace rectangles while keeping compatibility fields only where existing workflow tests still require them. Derive all shell rectangles in `ComputeLayout`; do not calculate widths independently in render or mouse code. Add a pure `layoutNavigation(layout, views, active)` primitive returning ordered `{View, Label, Rect}` items for full rail, compact rail, and narrow navigation. Both rendering and hit testing must consume that exact ordered result; neither may recalculate item positions. Reserve fixed overlay title/action rows and scroll only the overlay body.

- [ ] **Step 4: Run focused layout tests**

Run: `go test ./internal/tui -run 'TestModernShell|TestLayoutResponsiveRectsStayInsideTerminal|TestLayoutVisibleRange' -count=1`

Expected: PASS.

### Task 3: Attention-first overview and modern shell rendering

**Files:**
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/rows.go`
- Modify: `internal/tui/render_test.go`
- Modify: `internal/tui/foundation_test.go`

- [ ] **Step 1: Write failing semantic render tests**

Assert that Overview renders:

- a restrained app bar;
- Skills, Updates, Unmanaged, and Issues metrics;
- `Not checked` when the offline snapshot contains no update report;
- `Needs attention` ordered by recovery/error, conflict/drift, unmanaged/update, then stable key;
- no SKILL.md, hashes, file tree, or long path on the first screen;
- stable selected key and scroll anchor as inventory events merge.

- [ ] **Step 2: Run the overview tests and verify RED**

Run: `go test ./internal/tui -run 'TestModernOverview|TestStartupIncrementalMergePreservesIdentitySelectionAndScroll' -count=1`

Expected: FAIL because the old overview is a three-row summary with horizontal tabs.

- [ ] **Step 3: Implement shell and overview rendering**

Render the ordered result of `layoutNavigation` inside the navigation rectangle, then render a single main workspace. Use compact metric labels rather than boxed cards. Build attention rows from existing snapshot/inventory data only; do not read updatecheck cache files and do not invoke update checking or any service method. Sort with a pure severity/key comparator.

- [ ] **Step 4: Prove startup remains offline**

Run: `go test ./internal/tui -run 'TestStartupSnapshotIsOfflineAndInventoryIsIncremental|TestModernOverview' -count=1`

Expected: PASS and the counting update/network fake remains at zero calls.

### Task 4: Two-line collections and progressive details

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/rows.go`
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/render_test.go`

- [ ] **Step 1: Write failing collection tests**

For Library, Workspaces, Presets, Migration, and Status, assert each visible item has a primary name/state line and a secondary source/scope/context line. Assert the selected item has a non-color marker, detail groups are headed Summary/Usage/Files/Diagnostics only when relevant, and no more than three primary actions are visible outside More.

- [ ] **Step 2: Run collection tests and verify RED**

Run: `go test ./internal/tui -run 'TestTwoLineCollections|TestProgressiveDetails|TestPrimaryAction' -count=1`

Expected: FAIL because rows are currently one terminal line and detail output is ungrouped.

- [ ] **Step 3: Implement semantic row rendering**

Keep row identity and filtering unchanged. Add presentation helpers for badge class and secondary context. Make visible capacity account for two physical lines per logical row. Preserve cursor/selection by `selectionKey`, not rendered index.

- [ ] **Step 4: Run focused collection and workflow tests**

Run: `go test ./internal/tui -run 'TestTwoLineCollections|TestProgressiveDetails|TestPreset|TestProject|TestMigration|TestStatus' -count=1`

Expected: PASS.

### Task 5: Exact mouse geometry and compact overlays

**Files:**
- Modify: `internal/tui/input_mouse.go`
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/input_mouse_test.go`
- Modify: `internal/tui/action_bar_layout_test.go`

- [ ] **Step 1: Write failing hit-region tests**

Test full/compact/narrow navigation, both physical lines of every visible item, row selection marker, scroll mapping, action buttons, overlay Confirm/Cancel, and blank separators. Each mouse route must produce the same model/request as the existing keyboard action dispatcher.

- [ ] **Step 2: Run mouse tests and verify RED**

Run: `go test ./internal/tui -run 'TestModernMouseGeometry|TestMouseTabsRowsCheckboxWheelAndKeyboardParity|TestRenderedActionBarClickBoundariesAndWhitespace' -count=1`

Expected: FAIL because hit regions still assume one-line rows and horizontal tabs.

- [ ] **Step 3: Derive hit regions from rendered geometry**

Use only rectangles from `ComputeLayout`, `layoutNavigation`, `VisibleRange`, and `layoutActionBar`. A two-line row has one shared click target; checkbox/select markers get an exact sub-rectangle. Overlay clicks never leak to navigation or content. Keep Busy/MutationBusy gates unchanged.

- [ ] **Step 4: Run focused parity tests**

Run: `go test ./internal/tui -run 'TestModernMouseGeometry|TestMouse|TestRenderedActionBar|TestPrimaryActionButtonsHaveKeyboardAndMouseParity' -count=1`

Expected: PASS.

### Task 6: Visual regression matrix and full verification

**Files:**
- Modify: `internal/tui/render_test.go`
- Modify: `internal/tui/layout_test.go`
- Modify: `internal/tui/foundation_blockers_test.go`

- [ ] **Step 1: Add the final semantic matrix**

Render healthy, empty, loading, partial, warning, error, confirm, input, More, and Configuration states at the required width/height matrix. Strip ANSI and assert landmarks, focus marker, severity label, bounded line width, and always-visible short-height actions.

Also create the named table-driven `TestVisualInspectionFixtures` in `internal/tui/render_test.go`. Its cases must explicitly construct dark, light, reduced-color, and no-color themes (no ambient-terminal detection), render widths 120, 80, 59, and 38, and use `t.Logf` to emit both ANSI and `stripANSI` output. Assert the case table ran all four modes so `-run TestVisualInspectionFixtures` cannot pass on an empty match.

- [ ] **Step 2: Run all TUI tests**

Run: `go test ./internal/tui -count=1`

Expected: PASS.

- [ ] **Step 3: Run race and static verification**

Run: `go test -race ./internal/tui -count=1`

Run: `go test -race ./... -count=1`

Run: `go vet ./...`

Expected: PASS.

- [ ] **Step 4: Run repository regression verification**

Run: `go test ./... -count=1`

Run: `go build ./...`

Run: `GOOS=windows GOARCH=amd64 go build ./...`

Run: `GOCACHE=/tmp/aikit-modern-tui-e2e-cache make test-e2e`

Expected: PASS. Startup network counters remain zero and no application request/result contract changes.

- [ ] **Step 5: Inspect dark, light, reduced, and no-color renders**

Run the deterministic render fixture test with `-v`. It explicitly constructs dark, light, reduced, and no-color modes rather than relying on the invoking terminal. Inspect every emitted ANSI/stripped fixture at widths 120, 80, 59, and 38 and confirm navigation, selection, severity, focus, overlay title, and overlay actions remain unambiguous without clipping.

Run: `go test ./internal/tui -run TestVisualInspectionFixtures -count=1 -v`

Expected: PASS; the visual fixture output satisfies the approved spec in all four palette modes.

- [ ] **Step 6: Inspect the final patch**

Run: `git diff --check`

Run: `git diff -- internal/tui cmd/root.go docs/superpowers/specs/2026-08-14-modern-tui-visual-design.md docs/superpowers/plans/2026-08-14-modern-tui-visual-redesign.md`

Expected: no whitespace errors; only presentation/layout/tests plus the approved spec and plan changed for this redesign.
