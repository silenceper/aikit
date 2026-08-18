# Library Multi-select Action Bar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the hidden Library multi-select workflow with a visible, responsive, keyboard-and-mouse-equivalent contextual action bar for atomic Enable, Disable, Update, Remove, and local Clear operations.

**Architecture:** Add a Library-only typed action projection that owns stable IDs, labels, mnemonics, enabled state, and exact disabled reasons. Project it into the existing collection action bar through one count-aware layout primitive used by both rendering and hit testing; dispatch selection actions by ID/index before legacy string actions. Reuse the existing `PreviewBatch`, exact scope picker, Force confirmation, `Batch`, semantic activity, and result handlers without changing any backend API.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, existing `internal/tui` layout/action primitives, existing `internal/app` batch contracts, table-driven Go tests.

---

## File structure

- Create `internal/tui/library_selection_actions.go`: typed action IDs, projection, eligibility, display labels, mnemonic lookup, and ID-based dispatch helpers.
- Create `internal/tui/library_selection_actions_test.go`: projection, disabled reasons, exact batch request, geometry, parity, and lifecycle tests.
- Modify `internal/tui/action_bar.go`: count-aware contextual bar geometry composed from the existing paged action bar.
- Modify `internal/tui/action_bar_layout_test.go`: responsive one-line and pure-mouse pagination coverage.
- Modify `internal/tui/pane_actions.go`: switch the Library collection pane to the selection projection when non-empty.
- Modify `internal/tui/render.go`: mnemonic and disabled presentation without changing pane height.
- Modify `internal/tui/input_mouse.go`: use the same contextual layout and action indexes for hit testing.
- Modify `internal/tui/input_keyboard.go`: mnemonic precedence, disabled dispatch, and selection-aware Esc.
- Modify `internal/tui/actions.go`: ID-based dispatch and existing exact scope/preview reuse.
- Modify `internal/tui/scope_picker.go`: add a Library batch picker purpose that exposes only single-binding exact scopes.
- Modify `internal/tui/model.go`: retain selection on batch error/partial; clear only on complete success.
- Modify `internal/tui/styles.go`: mnemonic and disabled styles with a non-color fallback.
- Modify existing focused tests only where they intentionally assert the old hidden-`More` workflow.

### Task 1: Typed Library selection action projection

**Files:**
- Create: `internal/tui/library_selection_actions.go`
- Create: `internal/tui/library_selection_actions_test.go`
- Reference: `internal/tui/actions.go:827-915`
- Reference: `internal/tui/model.go:90-109`

- [ ] **Step 1: Write the failing projection and eligibility tests**

Add table-driven tests for zero, one, and multiple selected Library skills:

```go
actions := model.librarySelectionActions()
assertActionIDs(t, actions,
    selectionEnable, selectionDisable, selectionUpdate, selectionRemove, selectionClear)
```

Verify Update is disabled for a local source, missing/non-branch ref, missing result, `check-failed`, empty current/remote, and `Current != skill.Resolved`. The reason must name the first stable selected skill and exact condition. Verify a complete `update-available` result enables Update and `libraryBatchRequest(BatchUpdate)` copies `Ref` from the ledger and `Resolved`/`Remote` from the update record.

- [ ] **Step 2: Run focused tests and observe RED**

```bash
go test ./internal/tui -run 'TestLibrarySelectionActions|TestLibrarySelectionUpdateEligibility' -count=1 -v
```

Expected: FAIL because the typed action projection does not exist.

- [ ] **Step 3: Implement the minimal typed projection**

```go
type librarySelectionActionID string

const (
    selectionEnable  librarySelectionActionID = "enable"
    selectionDisable librarySelectionActionID = "disable"
    selectionUpdate  librarySelectionActionID = "update"
    selectionRemove  librarySelectionActionID = "remove"
    selectionClear   librarySelectionActionID = "clear"
)

type librarySelectionAction struct {
    ID       librarySelectionActionID
    Label    string
    Mnemonic rune
    Enabled  bool
    Reason   string
}
```

`librarySelectionActions` returns nil for zero selected Library skills and the five ordered descriptors otherwise. Reuse one eligibility helper from both the projection and `libraryBatchRequest(BatchUpdate)` so presentation and request construction cannot drift. Preserve stable skill-ID ordering.

- [ ] **Step 4: Run focused and package tests**

```bash
go test ./internal/tui -run 'TestLibrarySelectionActions|TestLibrarySelectionUpdateEligibility' -count=1 -v
go test ./internal/tui -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/library_selection_actions.go internal/tui/library_selection_actions_test.go internal/tui/actions.go
git commit -m "feat(tui): model Library selection actions"
```

### Task 2: Contextual single-line rendering and shared geometry

**Files:**
- Modify: `internal/tui/action_bar.go`
- Modify: `internal/tui/action_bar_layout_test.go`
- Modify: `internal/tui/pane_actions.go`
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/input_mouse.go`
- Modify: `internal/tui/styles.go`

- [ ] **Step 1: Write failing render and geometry tests**

For widths 24, 38, 59, 80, and 120, assert:

- zero selection still shows `Add source` and `More`;
- selected state shows `N selected` and all five actions, without `Add source`/`More`;
- the contextual row is one display line and collection/detail rectangles are unchanged;
- the count has no hit region;
- every visible action's first and last display cell maps to its typed action index;
- separators, clipped cells, and unused space are no-ops;
- keyboard paging and pure mouse page controls/wheel can reach Clear from default index zero at 24 columns; and
- long ASCII, CJK, and emoji row names remain bounded with the state column visible.
- partial/error BatchResult detail at all target widths does not replace the
  retained Library selection bar in render or hit geometry.

- [ ] **Step 2: Run geometry tests and observe RED**

```bash
go test ./internal/tui -run 'TestLibrarySelectionBar|TestLibrarySelectionBarResponsiveGeometry' -count=1 -v
```

Expected: FAIL because the selection state still uses normal collection actions.

- [ ] **Step 3: Implement one count-aware contextual layout**

Compose `layoutActionBar` rather than duplicating it:

```go
type librarySelectionBarLayout struct {
    Text    string
    Count   Rect
    Actions actionBarLayout
}
```

Reserve display cells for `N selected`, lay out paged actions in the remaining width, and translate action/page rectangles. At extreme widths retain the focused action and a reachable page control; never wrap. Rendering and `hitRegions()` call the same helper.

When selection is non-empty, Library `collectionActions()` returns typed labels instead of `Add source`/`More`. Render the mnemonic character inside each word using the semantic theme and disabled labels dimmed. `NO_COLOR` retains underline/focus markers. Do not change `ComputeLayout`.

In compact layouts, make a non-empty Library selection bar take precedence over
pinned BatchResult detail actions in both `renderFramedShell` and `hitRegions`.
Do not hide the result itself; only preserve collection action ownership so a
failed/partial batch remains retryable.

- [ ] **Step 4: Run geometry, rendering, mouse, and package tests**

```bash
go test ./internal/tui -run 'TestLibrarySelectionBar|TestLibrarySelectionBarResponsiveGeometry|TestActionBar' -count=1 -v
go test ./internal/tui -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/action_bar.go internal/tui/action_bar_layout_test.go internal/tui/pane_actions.go internal/tui/render.go internal/tui/input_mouse.go internal/tui/styles.go
git commit -m "feat(tui): show contextual Library batch actions"
```

### Task 3: Keyboard, mouse, disabled feedback, and Escape behavior

**Files:**
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/input_mouse.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/scope_picker.go`
- Modify: `internal/tui/library_selection_actions_test.go`

- [ ] **Step 1: Write failing end-to-end input tests**

Use real `tea.KeyMsg` and `tea.MouseMsg` updates to prove:

- `e`, `d`, `u`, `r`, and `c` in Library table mode with selection dispatch the same typed action as focused Enter and the matching mouse button;
- mnemonics do not override filter/input/overlay/detail behavior or ordinary row shortcuts with zero selection;
- disabled Update via mnemonic, Enter, or mouse makes zero `PreviewBatch`/`Batch` calls and reports the same exact reason;
- Enable/Disable enter the exact scope picker and do not submit before scope preview and confirmation;
- the Library batch scope picker never contains `All agents`, every choice has
  exactly one Binding, and every selected skill is expanded into that one scope;
- Clear performs no backend call; and
- Esc closes overlay/detail first, then clears selection while staying in Library, then performs normal back on the next Esc.

- [ ] **Step 2: Run focused input tests and observe RED**

```bash
go test ./internal/tui -run 'TestLibrarySelectionMnemonicParity|TestDisabledLibrarySelectionAction|TestLibrarySelectionEscape' -count=1 -v
```

Expected: FAIL because mnemonic and selection-aware cancellation are absent.

- [ ] **Step 3: Implement ID-based shared dispatch**

Before legacy label dispatch in `performPrimaryAction`, detect the Library contextual bar and map the selected index to a descriptor ID. Disabled descriptors return no command after setting only the ordinary replaceable `Status` hint; they must not create `ActivityWarning` or a `ReviewTarget`. Selection changes, Clear, or the next enabled action replace that hint. Enabled IDs reuse `libraryBatchRequest`, `batchPreviewCmd`, and local clear paths.

Add a dedicated `pickerLibraryBatchScope` purpose. Its choices are the existing
global-agent, project-common, and project-agent entries filtered to exactly one
Binding, explicitly excluding `All agents`. Applying it expands the one chosen
binding across the complete selected stable-ID set and then issues one
`PreviewBatch`.

In `updateKey`, after overlay/input/filter/detail guards and before ordinary table shortcuts, map `e/d/u/r/c` to the same descriptor dispatch. Add Library selection clearing to `cancel()` after overlay/detail cancellation but before route/back behavior.

- [ ] **Step 4: Run focused, parity, and package tests**

```bash
go test ./internal/tui -run 'TestLibrarySelectionMnemonicParity|TestDisabledLibrarySelectionAction|TestLibrarySelectionEscape|KeyboardAndMouseParity' -count=1 -v
go test ./internal/tui -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/input_keyboard.go internal/tui/input_mouse.go internal/tui/actions.go internal/tui/scope_picker.go internal/tui/library_selection_actions_test.go
git commit -m "feat(tui): unify Library batch input"
```

### Task 4: Exact preview, Force, and result selection lifecycle

**Files:**
- Modify: `internal/tui/library_selection_actions_test.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/library_task6a_test.go`

- [ ] **Step 1: Write failing backend-contract tests**

Record fake-service requests and assert:

- Remove sends exactly one `PreviewBatch(BatchRemove)` with the complete stable selected ID set and never calls `PreviewRemove`;
- `RequiresForce` enters a second Force confirmation and only the final confirmation submits one `Batch{Force:true}`;
- Update sends one complete exact-token `PreviewBatch` and then one atomic `Batch`;
- cancel at scope, preview, Force, or final confirmation performs zero mutation calls;
- complete success clears selection;
- top-level error, `ExitPartial`, or typed issues retain selection and typed batch detail; and
- `MutationBusy` prevents keyboard and mouse duplicate submission.

- [ ] **Step 2: Run contract tests and observe RED**

```bash
go test ./internal/tui -run 'TestLibrarySelectionBatchRemove|TestLibrarySelectionBatchUpdate|TestLibrarySelectionResultLifecycle' -count=1 -v
```

Expected: lifecycle tests FAIL because `batchOperationMsg` currently clears `Selected` before checking the result.

- [ ] **Step 3: Correct lifecycle without changing app contracts**

Keep the existing single `PreviewBatch` and `Batch` commands. In `batchOperationMsg`, clear selection only after nil error, `Exit != ExitPartial`, and no typed issues. Retain selection for errors and partial results before opening recovery or details. Assign `BatchResult` before every branch and preserve aggregate `Changed` as the durable-change authority.

- [ ] **Step 4: Run focused and package tests**

```bash
go test ./internal/tui -run 'TestLibrarySelectionBatchRemove|TestLibrarySelectionBatchUpdate|TestLibrarySelectionResultLifecycle|TestBatch' -count=1 -v
go test ./internal/tui -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/library_selection_actions_test.go internal/tui/library_task6a_test.go
git commit -m "fix(tui): preserve failed Library batch selections"
```

### Task 5: Regression and release verification

**Files:**
- Modify only if an in-scope TUI regression is exposed.

- [ ] **Step 1: Run formatting and diff validation**

```bash
gofmt -w internal/tui
git diff --check
test -z "$(gofmt -l internal/tui)"
```

Expected: no diff-check or gofmt listing output.

- [ ] **Step 2: Run focused and full native verification**

```bash
go test ./internal/tui -count=1
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
```

Expected: all commands exit 0.

- [ ] **Step 3: Run cross-platform and E2E verification**

```bash
GOOS=linux GOARCH=amd64 go build ./...
GOOS=windows GOARCH=amd64 go build ./...
GOCACHE=/tmp/aikit-library-selection-e2e-cache make test-e2e
```

Expected: builds exit 0 and E2E prints `aikit local e2e: PASS`.

- [ ] **Step 4: Review final scope**

```bash
git status --short
git diff --stat
git diff -- internal/tui
```

Expected: production changes are confined to `internal/tui`; no app API, config schema, filesystem/recovery, CLI, or web changes.

- [ ] **Step 5: Commit final in-scope adjustments if any**

```bash
git add internal/tui
git commit -m "test(tui): verify Library multi-select actions"
```
