# Configuration Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the blocking Configuration modal with a normal, navigable, read-only TUI tool page.

**Architecture:** Model Configuration as `ViewConfiguration`, route it through the existing `navigationView` destination path, and render its paths and validation state through the normal collection/detail panes. Keep `ModeErrorDetail` for Show paths, retain the existing typed app commands, and let late async results update cached state without changing `ActiveView`.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, existing `internal/tui` model/layout/action primitives, table-driven Go tests.

---

## File structure

- Modify `internal/tui/model.go`: add the normal Configuration view and TUI-local validation display state; retire Configuration as an overlay mode.
- Modify `internal/tui/navigation.go`: route Configuration through `navigationView` and the shared destination state machine.
- Modify `internal/tui/input_keyboard.go`: make `Ctrl+K` enter the normal page and remove modal-only Configuration key handling.
- Modify `internal/tui/input_mouse.go`: remove Configuration from overlay capture while retaining shared page action hit testing.
- Modify `internal/tui/pane_actions.go`: expose page-owned Configuration actions.
- Modify `internal/tui/rows.go`: project config/library/cache and validation state into stable normal-page rows.
- Modify `internal/tui/render.go`, `internal/tui/overlays.go`: render Configuration through framed panes, not the overlay renderer.
- Modify `internal/tui/modal.go`, `internal/tui/actions.go`, `internal/tui/activity.go`: preserve Show paths return state and validation result semantics.
- Modify `internal/tui/configuration_test.go`, `internal/tui/navigation_test.go`, `internal/tui/foundation_blockers_test.go`, and affected presentation tests: replace modal assumptions with normal-page acceptance coverage.

### Task 1: Route Configuration as a normal page

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/navigation.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/input_mouse.go`
- Test: `internal/tui/configuration_test.go`
- Test: `internal/tui/navigation_test.go`

- [ ] **Step 1: Write failing normal-route tests**

Add tests that activate Configuration through both a real navigation mouse click and keyboard navigation, then assert:

```go
if got.ActiveView != ViewConfiguration || got.Mode != ModeTable || got.hasOverlay() {
    t.Fatalf("configuration route view=%s mode=%s overlay=%v", got.ActiveView, got.Mode, got.hasOverlay())
}
```

From that state, click Library and activate Presets with keyboard; assert immediate route changes. Open Configuration through the real `Model.Update` path and assert it immediately enters a generation-tagged `ActivityReading` state with `Busy=true`. While that real load is in flight:

- keyboard and mouse navigation to another destination still works;
- activating Configuration again returns no command;
- Validate and Reload cannot submit another command;
- delivering the late generation-tagged load result updates cached detail/activity but does not change the selected destination.

- [ ] **Step 2: Run the focused tests and verify RED**

Run:

```bash
GOCACHE=/tmp/aikit-config-page-red go test ./internal/tui -run 'TestConfiguration(NormalRoute|NavigationWhileReading)' -count=1
```

Expected: FAIL because Configuration still enters `ModeConfiguration`, `hasOverlay()` is true, and its navigation entry is not a `navigationView`.

- [ ] **Step 3: Implement the normal route**

Make the minimal state changes:

```go
const ViewConfiguration View = "configuration"

{Key: "configuration", Label: "Configuration", Section: "Tools", Kind: navigationView, View: ViewConfiguration}
```

Have activation call `switchDestination(entry)`, set `Busy=true` plus a specific `Loading configuration paths...` status, and return `configurationCmd`. This lets the existing `Model.Update` transition wrapper create the `ActivityReading` generation envelope. When Configuration is already the active destination and a read is in flight, activation returns no command; the general reading-state keyboard/mouse handlers continue to allow other `navigationView` destinations. Change `Ctrl+K` to the same helper. Remove `ModeConfiguration` from `hasOverlay`, overlay-only keyboard/mouse branches, and modal entry helpers. Do not change genuine overlay behavior.

- [ ] **Step 4: Run focused and package tests**

Run:

```bash
GOCACHE=/tmp/aikit-config-page-green go test ./internal/tui -run 'TestConfiguration(NormalRoute|NavigationWhileReading)' -count=1
GOCACHE=/tmp/aikit-config-page-green go test ./internal/tui -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/navigation.go internal/tui/input_keyboard.go internal/tui/input_mouse.go internal/tui/modal.go internal/tui/configuration_test.go internal/tui/navigation_test.go
git commit -m "refactor(tui): route Configuration as a page"
```

### Task 2: Render page content and page-owned actions

**Files:**
- Modify: `internal/tui/rows.go`
- Modify: `internal/tui/pane_actions.go`
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/actions.go`
- Test: `internal/tui/configuration_test.go`
- Test: `internal/tui/action_bar_layout_test.go`
- Test: `internal/tui/render_test.go`

- [ ] **Step 1: Write failing page presentation tests**

Cover wide, compact, narrow, and low-height layouts. Assert Configuration renders in the framed workspace, the navigation remains visible, no overlay panel covers the workspace, long paths remain bounded, and the page actions are exactly:

```go
[]string{"Validate", "Reload", "Show paths"}
```

Use real mouse hit regions and keyboard focus traversal to invoke every action. Assert Show paths enters `ModeErrorDetail`, and closing it restores `ViewConfiguration` plus the prior page focus.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
GOCACHE=/tmp/aikit-config-present-red go test ./internal/tui -run 'TestConfiguration(PagePresentation|PageActions|Responsive)' -count=1
```

Expected: FAIL because content/actions are still produced only by modal code.

- [ ] **Step 3: Implement normal-page projection**

Project stable page rows:

```go
[]row{
    {Key: "configuration:config", Name: "Config file", Source: m.Config.Config, State: "Read only"},
    {Key: "configuration:library", Name: "Library", Source: m.Config.Library, State: "Managed content"},
    {Key: "configuration:cache", Name: "Cache", Source: m.Config.Cache, State: "Git metadata"},
    {Key: "configuration:validation", Name: "Validation", State: m.configurationValidationLabel(), Detail: m.ConfigValidationDisplay.Message},
}
```

Add page-owned collection actions in `collectionActions`. Remove Configuration content from `overlayLines`, remove the modal `Close` action, and let the shared page action registry dispatch Validate, Reload, and Show paths. Reuse display-cell-aware clipping/wrapping and the shared action-bar geometry.

- [ ] **Step 4: Run focused and package tests**

Run:

```bash
GOCACHE=/tmp/aikit-config-present-green go test ./internal/tui -run 'TestConfiguration(PagePresentation|PageActions|Responsive)' -count=1
GOCACHE=/tmp/aikit-config-present-green go test ./internal/tui -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/rows.go internal/tui/pane_actions.go internal/tui/render.go internal/tui/overlays.go internal/tui/actions.go internal/tui/configuration_test.go internal/tui/action_bar_layout_test.go internal/tui/render_test.go
git commit -m "feat(tui): render Configuration tool page"
```

### Task 3: Preserve validation state and async navigation

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/activity.go`
- Modify: `internal/tui/configuration_test.go`
- Modify: `internal/tui/activity_integration_test.go`

- [ ] **Step 1: Write failing validation lifecycle tests**

Add a TUI-only state:

```go
type configurationValidationDisplay struct {
    Attempted bool
    Valid     bool
    Message   string
}
```

Test all four states:

- first entry: `Not validated`;
- successful validation: `Valid` and path/message retained;
- failed validation: `Invalid` and exact error retained;
- navigate away and back: last result remains.

For load, Validate, and Reload, start each command through the real `Model.Update` activity path, navigate elsewhere using keyboard and mouse before delivering its generation-tagged result, then deliver the late result. Assert cached configuration/validation updates and Activity review/terminal state complete normally, but `ActiveView` remains the destination chosen by the user. Also assert a second activation/action during the in-flight read produces no command and no second backend call.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
GOCACHE=/tmp/aikit-config-state-red go test ./internal/tui -run 'TestConfiguration(ValidationLifecycle|LateResultKeepsRoute)' -count=1
```

Expected: FAIL because validation attempt/error state currently lives only in replaceable global `Err`/`Status`.

- [ ] **Step 3: Implement durable session display state**

Update `configurationValidationMsg` handling before general activity completion clears or replaces status:

```go
m.ConfigValidationDisplay.Attempted = true
m.ConfigValidationDisplay.Valid = msg.err == nil && msg.result.Valid
m.ConfigValidationDisplay.Message = firstNonEmpty(errorText(msg.err), msg.result.Path)
```

Do not set `ActiveView` or `Mode` in configuration result handlers. Preserve typed activity/error review and keep duplicate Configuration action submission blocked while its read is active.

- [ ] **Step 4: Run focused, TUI, and integration tests**

Run:

```bash
GOCACHE=/tmp/aikit-config-state-green go test ./internal/tui -run 'TestConfiguration(ValidationLifecycle|LateResultKeepsRoute)' -count=1
GOCACHE=/tmp/aikit-config-state-green go test ./internal/tui ./cmd -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/activity.go internal/tui/configuration_test.go internal/tui/activity_integration_test.go
git commit -m "fix(tui): preserve Configuration read state"
```

### Task 4: Remove obsolete modal assumptions and verify release quality

**Files:**
- Modify: `internal/tui/foundation_blockers_test.go`
- Modify: `internal/tui/modal_entry_test.go`
- Modify: `internal/tui/input_mouse_test.go`
- Modify: any existing Configuration fixture that still constructs `ModeConfiguration`

- [ ] **Step 1: Audit and update obsolete tests**

Run:

```bash
rg -n 'ModeConfiguration|enterConfiguration|navigationConfiguration|Esc Close' internal/tui
```

Remove production occurrences and convert tests to `ViewConfiguration` normal-page fixtures. Keep real overlay capture tests unchanged.

- [ ] **Step 2: Run formatting and static checks**

Run:

```bash
gofmt -w internal/tui
test -z "$(gofmt -l internal/tui)"
git diff --check
GOCACHE=/tmp/aikit-config-vet GOPROXY=off go vet ./...
```

Expected: all exit 0.

- [ ] **Step 3: Run full verification**

Run:

```bash
GOCACHE=/tmp/aikit-config-test go test ./... -count=1
GOCACHE=/tmp/aikit-config-race go test -race ./... -count=1
GOCACHE=/tmp/aikit-config-build GOPROXY=off go build ./...
GOCACHE=/tmp/aikit-config-linux GOPROXY=off GOOS=linux GOARCH=amd64 go build ./...
GOCACHE=/tmp/aikit-config-windows GOPROXY=off GOOS=windows GOARCH=amd64 go build ./...
GOCACHE=/tmp/aikit-config-e2e make test-e2e
```

Expected: all pass; E2E prints `aikit local e2e: PASS`.

- [ ] **Step 4: Request independent code review**

Ask the reviewer to verify normal routing, in-flight navigation, late-result behavior, genuine overlay capture, narrow mouse reachability, and validation-state persistence. Fix all Critical/Important findings and rerun verification.

- [ ] **Step 5: Commit final test migrations if necessary**

```bash
git add internal/tui
git commit -m "test(tui): verify Configuration page navigation"
```
