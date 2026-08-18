# Activity Status Bar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the visible generic `busy` state with a colored, truthful activity status bar while allowing browsing during read-only work and preserving mutation safety.

**Architecture:** Add a presentation-only `Activity` state machine to the Bubble Tea model. Existing typed app results remain authoritative; commands explicitly start and finish activities, while generation-tagged spinner/expiry messages prevent stale UI updates. Keyboard and mouse share semantic browsing/submission gates, and reviewable results use a closed `ReviewTarget` plus conditional status focus.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, existing `internal/tui` semantic theme and typed `internal/app` results.

---

## File map

- Create `internal/tui/activity.go`: activity kinds, review targets, transitions, spinner/expiry messages, rendering text, and shared input classification.
- Create `internal/tui/activity_test.go`: pure state-machine, timer-generation, marker/color, clipping, and no-fake-progress tests.
- Create `internal/tui/activity_integration_test.go`: command lifecycle, browsing/submission gates, status focus, keyboard/mouse parity, and typed review routing.
- Modify `internal/tui/model.go`: store activity/focus history, handle activity messages, and translate command results into terminal activities.
- Modify `internal/tui/styles.go`: add blue reading, cyan network, and purple mutating semantic styles while retaining adaptive/reduced/no-color behavior.
- Modify `internal/tui/render.go`: render activity in the framed footer and short phase in the app bar; remove visible `busy`.
- Modify `internal/tui/layout.go`: expose the actual footer status hit rectangle used by render and mouse.
- Modify `internal/tui/input_keyboard.go`: permit browsing during read-only activity, retain mutation gate, and support `FocusStatus`.
- Modify `internal/tui/input_mouse.go`: use the same gate and make reviewable status clickable.
- Modify `internal/tui/actions.go`, `internal/tui/overview.go`, `internal/tui/workspace_actions.go`, `internal/tui/scope_picker.go`: replace ad-hoc start labels with explicit read/network/mutation activity helpers.
- Modify focused existing TUI tests only where they assert the old literal `busy` or global read-only input lock.

### Task 1: Pure activity state and semantic theme

**Files:**
- Create: `internal/tui/activity.go`
- Create: `internal/tui/activity_test.go`
- Modify: `internal/tui/styles.go`

- [ ] **Step 1: Write failing activity model tests**

Cover all six kinds, exact `current/total`, indeterminate spinner text, item labels,
review target identity, `NO_COLOR`, reduced mode, and display-cell clipping at
24/38/80/120 columns. Assert no percentage appears without real totals.

```go
func TestActivityTextNeverInventsProgress(t *testing.T) {
    activity := Activity{Kind: ActivityNetwork, Label: "Checking updates"}
    got := stripANSI(renderActivity(activity, 38, newSemanticTheme(themeNoColor)))
    if strings.Contains(got, "%") || got != "⠋ Checking updates" { t.Fatal(got) }
}
```

- [ ] **Step 2: Run RED test**

Run: `go test ./internal/tui -run 'TestActivity' -count=1`

Expected: FAIL because `Activity`, kinds, review target, and renderer do not exist.

- [ ] **Step 3: Implement the minimal pure model**

Define closed enums and no callbacks:

```go
type ActivityKind uint8
const (ActivityIdle ActivityKind = iota; ActivityReading; ActivityNetwork; ActivityMutating; ActivitySuccess; ActivityWarning; ActivityError)
type ReviewKind uint8
type ReviewTarget struct { Kind ReviewKind; Key string }
type Activity struct { Kind ActivityKind; Label string; Item string; Current, Total int; Frame int; Generation uint64; Review ReviewTarget }
```

Add theme styles for reading/network/mutating. `renderActivity` selects
`⠋/●/✓/!/×`, appends `current/total` only for `total > 0`, and clips by display
cells. Reduced/no-color modes retain markers.

- [ ] **Step 4: Run GREEN test**

Run: `go test ./internal/tui -run 'TestActivity' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/activity.go internal/tui/activity_test.go internal/tui/styles.go
git commit -m "feat(tui): add semantic activity state"
```

### Task 2: Generation-safe spinner and success expiry

**Files:**
- Modify: `internal/tui/activity.go`
- Modify: `internal/tui/model.go`
- Test: `internal/tui/activity_test.go`

- [ ] **Step 1: Write failing timer tests**

Assert spinner tick advances only the matching generation; an old tick cannot
change a newer activity; success expiry fires after `3*time.Second`; an old
expiry cannot clear a newer warning or operation.

- [ ] **Step 2: Run RED test**

Run: `go test ./internal/tui -run 'TestActivity(Spinner|Expiry|Generation)' -count=1`

Expected: FAIL because messages and transitions are missing.

- [ ] **Step 3: Implement messages and helpers**

Add `activityTickMsg`, `activityExpireMsg`, `activityTickCmd`,
`activityExpireCmd`, `beginActivity`, `finishActivity`, and generation checks in
`Model.Update`. Schedule ticks only for reading/network indeterminate activity;
schedule expiry only for success.

- [ ] **Step 4: Run GREEN and package tests**

Run: `go test ./internal/tui -run 'TestActivity' -count=1`

Run: `go test ./internal/tui -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/activity.go internal/tui/activity_test.go internal/tui/model.go
git commit -m "feat(tui): animate and expire activity status"
```

### Task 3: Shared footer/app-bar geometry and semantic rendering

**Files:**
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/layout.go`
- Modify: `internal/tui/input_mouse.go`
- Test: `internal/tui/render_test.go`
- Test: `internal/tui/input_mouse_test.go`

- [ ] **Step 1: Write failing render/hit tests**

At 24/38/80/120 columns assert the status and shortcuts remain bounded, CJK and
emoji are grapheme-safe, the literal visible word `busy` never appears, the app
bar shows only `scanning/checking/applying`, and the mouse status rectangle
matches the rendered activity cells.

- [ ] **Step 2: Run RED test**

Run: `go test ./internal/tui -run 'TestActivity(Render|Layout|Mouse)|TestAppBarDoesNotRenderBusy' -count=1`

Expected: FAIL on old `busy` rendering and missing status hit geometry.

- [ ] **Step 3: Implement shared rendering**

Make `renderFooterBody` compose rendered activity and contextual shortcuts.
Activity wins on narrow terminals, with `Ctrl+Q` retained when it fits. Give
`FooterPanel.Body` a shared `ActivityStatus` rect and use it for mouse focus.
Remove both `busy` branches from app-bar/context rendering.

- [ ] **Step 4: Run GREEN and visual regressions**

Run: `go test ./internal/tui -run 'TestActivity(Render|Layout|Mouse)|TestAppBarDoesNotRenderBusy|TestVisual' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/render.go internal/tui/layout.go internal/tui/input_mouse.go internal/tui/render_test.go internal/tui/input_mouse_test.go
git commit -m "feat(tui): render semantic activity footer"
```

### Task 4: Browsing versus submission input gate

**Files:**
- Modify: `internal/tui/activity.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/input_mouse.go`
- Create: `internal/tui/activity_integration_test.go`

- [ ] **Step 1: Write failing keyboard/mouse matrix tests**

While reading/network activity is active, verify navigation shortcuts, arrows,
`j/k`, scrolling, filter editing, help, loaded detail toggle, Escape, status
focus, and Ctrl+Q work. Verify selection mutation, action buttons, previews,
refresh, confirm, and every new async submission are rejected. Under mutating
activity, retain the existing strict keyboard/mouse gate.

- [ ] **Step 2: Run RED test**

Run: `go test ./internal/tui -run 'TestActivity(ReadOnlyGate|MutationGate|KeyboardMouseParity)' -count=1`

Expected: FAIL because current `Busy` returns before all input.

- [ ] **Step 3: Implement one semantic classifier**

Add browsing/submitting classification for `uiAction` and shared helpers used by
both key and mouse dispatch. Remove early blanket `Busy` returns. A read-only
activity rejects submitting actions but accepts browsing; `MutationBusy`
continues to reject everything except completion, emergency exit, and window
updates.

- [ ] **Step 4: Run GREEN and existing Busy tests**

Run: `go test ./internal/tui -run 'TestActivity(ReadOnlyGate|MutationGate|KeyboardMouseParity)|TestMouseConfirmCancelBusyGate' -count=1`

Run: `go test ./internal/tui -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/activity.go internal/tui/actions.go internal/tui/input_keyboard.go internal/tui/input_mouse.go internal/tui/activity_integration_test.go
git commit -m "feat(tui): allow browsing during read-only work"
```

### Task 5: Status focus and exact typed review routing

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/activity.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/input_mouse.go`
- Modify: `internal/tui/navigation.go`
- Test: `internal/tui/activity_integration_test.go`

- [ ] **Step 1: Write failing focus/routing tests**

Cover conditional `FocusStatus`, Tab order, no focus stealing, mouse click,
Enter precedence, exact stable keys/operation IDs for each review kind, Escape
dismissal, fallback to the previously valid focus, banner acknowledgement, and
ordinary row Enter remaining unchanged.

- [ ] **Step 2: Run RED test**

Run: `go test ./internal/tui -run 'TestActivity(StatusFocus|ExactReview|Dismiss)' -count=1`

Expected: FAIL because `FocusStatus` and exact review dispatch do not exist.

- [ ] **Step 3: Implement closed review dispatch**

Store previous focus when entering status focus. Add status to the Tab cycle
only when `ReviewTarget` is non-empty. Dispatch the closed review kind to the
existing `enterErrorDetail`, batch/result/recovery/status/update/inventory
views using the stable key. Opening or Escape dismissal clears only the banner,
not typed model data, and restores the previous valid focus.

- [ ] **Step 4: Run GREEN and parity tests**

Run: `go test ./internal/tui -run 'TestActivity(StatusFocus|ExactReview|Dismiss)' -count=1`

Run: `go test ./internal/tui -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/activity.go internal/tui/model.go internal/tui/input_keyboard.go internal/tui/input_mouse.go internal/tui/navigation.go internal/tui/activity_integration_test.go
git commit -m "feat(tui): add reviewable activity focus"
```

### Task 6: Integrate every asynchronous lifecycle

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/input_mouse.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/overview.go`
- Modify: `internal/tui/workspace_actions.go`
- Modify: `internal/tui/scope_picker.go`
- Test: `internal/tui/activity_integration_test.go`
- Test: focused existing workflow tests

- [ ] **Step 1: Write failing lifecycle table tests**

Table-drive Snapshot, Inventory, detail, configuration, compare, preview,
update check, sync preview, recovery preview, add, project, preset, batch,
update/import/adopt, sync, and recovery completion. Assert correct kind/label,
real progress where available, success/warning/error result, review target, and
no stale overwrite. Assert partial results are warning rather than success.

- [ ] **Step 2: Run RED test**

Run: `go test ./internal/tui -run 'TestActivityLifecycle' -count=1`

Expected: FAIL because call sites still set only `Busy`/`Status`.

- [ ] **Step 3: Integrate read-only starts and completions**

Use reading for Snapshot/detail/configuration/compare/preview and network for
explicit update checks or network-enabled add discovery. Inventory updates
`current/total` from real root events. Clear the in-flight guard on every error,
cancel, and completion path.

- [ ] **Step 4: Integrate mutation starts and completions**

Use mutating labels for confirmed add/project/preset/batch/update/import/adopt,
sync, and recovery. Translate aggregate and per-item typed results: committed
success -> success, partial/issues -> warning with exact review target, error ->
error with the best typed target. Do not infer completion from an item-level
`Changed` when aggregate `Changed` is false.

- [ ] **Step 5: Run GREEN and package regressions**

Run: `go test ./internal/tui -run 'TestActivityLifecycle|TestOverview|TestProject|TestPreset|TestRecovery|TestBatch' -count=1`

Run: `go test ./internal/tui -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/model.go internal/tui/actions.go internal/tui/input_mouse.go internal/tui/input_keyboard.go internal/tui/overview.go internal/tui/workspace_actions.go internal/tui/scope_picker.go internal/tui/activity_integration_test.go
git commit -m "feat(tui): report asynchronous activity lifecycle"
```

### Task 7: Full verification and real-config read-only smoke test

**Files:**
- Modify tests only if a verification failure exposes a real regression

- [ ] **Step 1: Run focused and full verification**

```bash
GOCACHE=/tmp/aikit-activity-final GOTMPDIR=/tmp go test ./internal/tui -count=1
GOCACHE=/tmp/aikit-activity-final GOTMPDIR=/tmp go test ./... -count=1
GOCACHE=/tmp/aikit-activity-final GOTMPDIR=/tmp go test -race ./... -count=1
GOCACHE=/tmp/aikit-activity-final GOTMPDIR=/tmp go vet ./...
GOCACHE=/tmp/aikit-activity-final GOTMPDIR=/tmp go build ./...
GOCACHE=/tmp/aikit-activity-final GOTMPDIR=/tmp GOOS=windows GOARCH=amd64 go build ./...
GOCACHE=/tmp/aikit-activity-final GOTMPDIR=/tmp make test-e2e
git diff --check
test -z "$(gofmt -l internal/tui)"
```

Expected: all commands exit 0; sandbox module stat-cache warnings are acceptable
only when the build/test exit code is 0.

- [ ] **Step 2: Run real configuration smoke test**

Hash `~/.aikit` files, run `make run`, inspect reading/network/mutation preview,
navigate while a read-only preview is active, cancel before every mutation,
exercise warning/error status focus, exit with Ctrl+Q, and verify the before/after
hash is identical.

- [ ] **Step 3: Review requirements and working tree**

Re-read `docs/superpowers/specs/2026-08-18-activity-status-bar-design.md`, check
each verification item, inspect `git diff --check`, and confirm no unrelated
file is modified.

- [ ] **Step 4: Commit and push**

```bash
git add internal/tui docs/superpowers
git commit -m "test(tui): verify semantic activity status"
git push origin main
```
