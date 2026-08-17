# Three-pane K9s-style TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace aikit's mode-accumulated presentation with a responsive Lazygit-style three-pane workspace rendered with disciplined K9s-style frames, while preserving every typed preview, confirmation, and recovery boundary.

**Architecture:** Extend the existing pure `Layout` geometry with framed pane primitives consumed by rendering and mouse hit testing. Keep the current `Model`, app services, rows, and workflow commands authoritative; keyboard and mouse translate into shared semantic actions. Implement the redesign inside `internal/tui` unless a test proves a typed backend gap.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, `rivo/uniseg`, existing `internal/app` and `internal/migrate` interfaces.

**Specification:** `docs/superpowers/specs/2026-08-17-three-pane-k9s-tui-design.md`

---

## File structure

- Create `internal/tui/panel.go`: frame glyphs, panel geometry, title/body/action slots, shared render geometry.
- Create `internal/tui/panel_test.go`: Unicode/ASCII, clipping, focus, and short-height geometry.
- Create `internal/tui/navigation.go`: five permanent destinations, Tools, and command-palette entries.
- Create `internal/tui/navigation_test.go`: navigation and palette keyboard/mouse behavior.
- Modify `internal/tui/layout.go`: wide/compact/narrow and height-degraded rectangles.
- Modify `internal/tui/render.go`: framed shell, collections, details, overlays, and footer.
- Modify `internal/tui/rows.go`: one-line semantic collection rows and stable identities.
- Modify `internal/tui/input_keyboard.go` and `input_mouse.go`: shared pane/action/picker semantics.
- Modify `internal/tui/actions.go`: distinguish Library selection, picker choice, picker Apply, and Tools routes.
- Modify `internal/tui/model.go`: pane focus, palette state, and permanent navigation.

---

### Task 1: Separate Library selection from picker choice

**Files:**
- Modify: `internal/tui/scope_picker.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/input_mouse.go`
- Modify: `internal/tui/render.go`
- Test: `internal/tui/scope_picker_test.go`
- Test: `internal/tui/input_mouse_test.go`

- [ ] **Step 1: Write failing regression tests**

Enter `ModeScopePicker` and `ModePresetPicker` from Library workflows, then click a row different from the current cursor. Assert the click changes the chosen row, returns no `tea.Cmd`, and makes zero preview/mutation calls. Assert Space chooses, Enter chooses and focuses the action row, and only `[Apply]` creates the preview command. Keep main Library tests proving row click, checkbox click, and Space toggle exactly once at widths 120/80/59/38.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/tui -run 'Test(StructuredScopePicker|LibraryRowMouse|Picker)' -count=1`

Expected: FAIL because picker row click currently dispatches `uiToggle`/`choosePicker`, and Enter submits directly.

- [ ] **Step 3: Implement minimal semantic separation**

Add `Selected int` to `pickerState`, initialized to `-1`. Split current submission behavior into:

```go
func (m Model) chooseHighlightedPicker() Model
func (m Model) applyPicker() (tea.Model, tea.Cmd)
```

Row click and Space call `chooseHighlightedPicker`. Enter chooses and sets `FocusActions`. Rename the visible picker action from `Select` to `Apply`; only that action calls `applyPicker`. Evaluate picker ownership before the main Library whole-row toggle branch.

- [ ] **Step 4: Run GREEN**

Run: `go test ./internal/tui -run 'Test(StructuredScopePicker|LibraryRowMouse|Picker)' -count=1 && go test ./internal/tui -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

Run: `git add internal/tui && git commit -m "fix(tui): separate picker choice from library selection"`

---

### Task 2: Build shared framed three-pane geometry

**Files:**
- Create: `internal/tui/panel.go`
- Create: `internal/tui/panel_test.go`
- Modify: `internal/tui/layout.go`
- Modify: `internal/tui/layout_test.go`
- Modify: `internal/tui/row_layout.go`
- Modify: `internal/tui/overlay_layout.go`

- [ ] **Step 1: Write failing geometry tests**

Table-drive widths 120/96/95/80/60/59/38/24 and heights 30/16/12/11/8/7/5. Assert: wide has Navigation/Collection/Detail; compact has Navigation/Collection with detail page; narrow has one active pane and breadcrumb; heights 8-11 have one framed pane plus single-line app/footer; heights below 8 render only `terminal too short; need 8 rows`; no rectangles overlap or escape; title/body/action/border and mouse rects come from one panel layout; Unicode and ASCII glyphs have identical cell geometry.

For every below-8 layout, assert all row, checkbox, action, navigation, confirm, and cancel hit rectangles are empty. Key and mouse events may only resize, open help, perform back/cancel when a parent exists, or use the existing Ctrl+Q/Ctrl+C exit contract; no command/service call is returned.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/tui -run 'Test(ThreePane|Panel|LowHeight|Layout)' -count=1`

Expected: FAIL because the current layout has an unframed rail and only a list/detail split.

- [ ] **Step 3: Implement minimal panel primitives**

Create:

```go
type FrameGlyphs struct {
    TopLeft, TopRight, BottomLeft, BottomRight string
    Horizontal, Vertical string
}
type PanelLayout struct { Outer, Title, Body, Actions Rect }
func layoutPanel(outer Rect, actions bool) PanelLayout
func renderPanel(layout PanelLayout, title string, focused bool, body []string, action string, glyphs FrameGlyphs) []string
```

Extend `Layout` with `AppBar`, `NavigationPanel`, `CollectionPanel`, `DetailPanel`, `LowHeight`, and `TooShort`. Preserve temporary compatibility aliases while old render tests migrate. When `TooShort` is true, return no interactive content rectangles and let rendering own only the bounded warning. Use existing grapheme/display-cell clipping.

- [ ] **Step 4: Run GREEN**

Run: `go test ./internal/tui -run 'Test(ThreePane|Panel|LowHeight|Layout)' -count=1 && go test ./internal/tui -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

Run: `git add internal/tui && git commit -m "feat(tui): add responsive framed pane geometry"`

---

### Task 3: Render the K9s-style shell from shared geometry

**Files:**
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/styles.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/input_mouse.go`
- Test: `internal/tui/render_test.go`
- Test: `internal/tui/styles_test.go`
- Test: `internal/tui/input_mouse_test.go`

- [ ] **Step 1: Write failing render/hit tests**

Assert ANSI-stripped wide output includes framed App, Navigation, Collection, Detail, and Footer titles; low height includes only the active frame; overlays have one complete frame; rows/cards are not individually boxed. Assert the focused pane has a non-color marker. For every visible row/action, render and hit rectangles must match in wide, compact, narrow, height 8, and height 12 layouts.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/tui -run 'Test(FramedShell|FrameHit|OverlayFrame|SemanticTheme)' -count=1`

Expected: FAIL because rendering still uses `joinRail` and has no panel borders.

- [ ] **Step 3: Implement framed composition**

Make `render()` compose terminal cells from panel outputs. Frame app/footer at height >=12; collapse them to one line at 8-11. Keep rows unboxed. Focus only one pane strongly. Reuse `layoutPanel` for overlays with fixed title/actions and scrollable body. Make `hitRegions()` consume panel body/action rectangles rather than recomputing positions.

- [ ] **Step 4: Run GREEN**

Run: `go test ./internal/tui -run 'Test(FramedShell|FrameHit|OverlayFrame|SemanticTheme)' -count=1 && go test ./internal/tui -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

Run: `git add internal/tui && git commit -m "feat(tui): render K9s-style framed workspace"`

---

### Task 4: Install five permanent destinations and Tools command palette

**Files:**
- Create: `internal/tui/navigation.go`
- Create: `internal/tui/navigation_test.go`
- Modify: `internal/tui/navigation_layout.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/input_mouse.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/render.go`

- [ ] **Step 1: Write failing navigation tests**

Assert permanent order Overview/Library/Workspaces/Presets/Status and a separate Tools section with Migration/Configuration. Number keys 1-5 and mouse clicks select identical destinations. `Ctrl+P` and `:` open a local command palette; typing filters; arrows move; Enter activates; Esc restores prior context. Migration opens its existing view; Configuration opens the existing read-only overlay.

The palette also exposes typed workflow entries `Add source`, `Create project`, `Create preset`, and `Review recovery` when applicable. Selecting them must call the same semantic dispatcher as the visible action and only enter the existing input/preview/recovery-review surface. Assert zero mutation, zero checkpoint, zero Git runner, zero update checker, and zero network call until that workflow reaches its separately confirmed stage.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/tui -run 'Test(PermanentNavigation|ToolsNavigation|CommandPalette|NavigationParity)' -count=1`

Expected: FAIL because Migration is a permanent sixth view and no command palette exists.

- [ ] **Step 3: Implement typed navigation entries**

Create:

```go
type navigationKind int
const (
    navigationView navigationKind = iota
    navigationConfiguration
    navigationAction
)
type navigationEntry struct {
    Key, Label, Section string
    Kind navigationKind
    View View
    Action uiAction
}
```

Add `ModeCommand` with local draft/filter state. Keep `ViewMigration` internally but remove it from permanent `topViews`. Map Configuration to the existing overlay. Map `navigationAction` through the same `perform(uiAction)` dispatcher used by visible buttons; palette code must not call backend commands directly. Render Tools after a separator in the Navigation frame.

- [ ] **Step 4: Run GREEN**

Run: `go test ./internal/tui -run 'Test(PermanentNavigation|ToolsNavigation|CommandPalette|NavigationParity)' -count=1 && go test ./internal/tui -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

Run: `git add internal/tui && git commit -m "feat(tui): add task navigation and command palette"`

---

### Task 5: Adapt Library collection and bounded live detail

**Files:**
- Modify: `internal/tui/rows.go`
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/input_mouse.go`
- Test: `internal/tui/render_test.go`
- Test: `internal/tui/input_mouse_test.go`
- Test: `internal/tui/library_task6a_test.go`
- Test: `internal/tui/operability_regression_test.go`

- [ ] **Step 1: Write failing Library tests**

Assert a wide Library page has one-line Collection rows and live Detail with source/ref/state, Summary, Usage, and Diagnostics; no row has its own frame; Detail exposes at most three primary actions. Long real descriptions, paths, CJK, and emoji remain inside the Detail frame. `View SKILL.md` stays bounded and scrollable. Whole-row click, checkbox, and Space toggle once; Enter focuses Detail without mutation; filter/refresh preserve stable identity.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/tui -run 'Test(LibraryWorkspace|LibraryRowMouse|BoundedSkill|Detail)' -count=1`

Expected: FAIL because current collection projection is two-line and detail is not framed.

- [ ] **Step 3: Implement one-line rows and live detail**

Render checkbox/name/compact source-state columns on one clipped line. Keep complete information in Detail. Fit Summary first, then bounded Usage/Files/Diagnostics with explicit omitted counts and the existing `View SKILL.md` action. Consume typed row severity only.

- [ ] **Step 4: Run GREEN**

Run: `go test ./internal/tui -run 'Test(LibraryWorkspace|LibraryRowMouse|BoundedSkill|Detail)' -count=1 && go test ./internal/tui -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

Run: `git add internal/tui && git commit -m "feat(tui): add framed library workspace"`

---

### Task 6: Adapt Workspaces, Presets, Overview, Status, and Tools

**Files:**
- Modify: `internal/tui/rows.go`
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/workspace_projection.go`
- Modify: `internal/tui/workspace_actions.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/input_mouse.go`
- Test: `internal/tui/workspace_project_test.go`
- Test: `internal/tui/project_management_test.go`
- Test: `internal/tui/preset_task6c_test.go`
- Test: `internal/tui/recovery_status_test.go`
- Test: `internal/tui/operability_regression_test.go`

- [ ] **Step 1: Write failing workspace flow tests**

Cover Workspaces empty/non-empty `[Create project]`; project Common/agent scope rows; `[Add skill] [Apply preset] [More]`; Presets empty/non-empty `[Create preset]`; member checklist and exact target apply; Overview framed metrics/attention; Status full item errors, refresh, sync, and recovery; keyboard/mouse parity, cancel zero-write, Busy single-submit, and stable identity at wide/compact/narrow widths.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/tui -run 'Test(Workspace|Project|Preset|Overview|Status|Recovery)' -count=1`

Expected: FAIL where old placement and empty-state actions do not match the framed workspace.

- [ ] **Step 3: Implement domain projections in shared panes**

Keep typed commands unchanged. Collection frames own Create actions even with zero rows. Project scopes render in Detail and open existing scoped Library workflows. Presets show members/usage and exact targets. Overview/Status use Collection/Detail framing without nested cards. Migration and Configuration render through Tools.

- [ ] **Step 4: Run GREEN**

Run: `go test ./internal/tui -run 'Test(Workspace|Project|Preset|Overview|Status|Recovery)' -count=1 && go test ./internal/tui -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

Run: `git add internal/tui && git commit -m "feat(tui): unify project preset and status workspaces"`

---

### Task 7: Complete accessibility and release verification

**Files:**
- Modify: `internal/tui/foundation_test.go`
- Modify: `internal/tui/foundation_blockers_test.go`
- Modify: `internal/tui/views_test.go`
- Modify: `internal/tui/styles_test.go`
- Modify: `internal/tui/usability_audit_test.go`
- Modify: `internal/tui/operability_regression_test.go`
- Modify: `README.md` only if visible navigation/shortcuts are documented there.

- [ ] **Step 1: Add final acceptance matrix**

Cover widths 120/96/95/80/60/59/38/24, heights 30/16/12/11/8/7/5, dark/light/`NO_COLOR`/reduced/ASCII, CJK/emoji, and real-sized 200-line content. Assert rendered lines fit display-cell width, hit regions stay inside frames, visible actions have keyboard/mouse parity, and startup remains offline/read-only. At heights below 8, assert the exact bounded warning, empty actionable hit regions, and zero backend calls for all non-help/non-back/non-exit input.

- [ ] **Step 2: Run acceptance tests and fix only demonstrated gaps**

Run: `go test ./internal/tui -run 'Test(Responsive|Visual|Usability|Operability|NoColor|ASCII|Real)' -count=1`

Expected: PASS after minimal presentation corrections.

- [ ] **Step 3: Run full verification**

Run each command separately and require exit 0:

```bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
GOOS=windows GOARCH=amd64 go build ./...
make test-e2e
git diff --check
test -z "$(gofmt -l internal/tui)"
```

- [ ] **Step 4: Perform a real terminal smoke test**

Run `make run` in a PTY against the user's existing config without confirming any mutation. Verify Library click/toggle, picker selection, pane focus, Detail scroll, Workspaces/Presets actions, command palette, compact/narrow resize, Esc back, and Ctrl+Q exit.

- [ ] **Step 5: Commit**

Run: `git add internal/tui README.md && git commit -m "test(tui): verify framed workspace accessibility"`

---

## Execution notes

- Work directly on the current branch because the user explicitly requested this after committing the previous baseline.
- Follow strict RED/GREEN for every behavior change.
- Do not use `netcat`, `socat`, `psexec`, `nmap`, network scanners, or password cracking tools.
- Do not add startup network activity or automatic mutation.
- Do not modify `internal/web` or reintroduce a Web UI.
- Stop if a requirement cannot be expressed through existing typed app/migration services rather than duplicating business planning in TUI code.
