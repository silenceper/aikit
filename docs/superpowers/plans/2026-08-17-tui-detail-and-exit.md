# TUI Detail Boundaries and Safe Exit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep real-world Library details bounded and make accidental TUI exit impossible without `Ctrl+Q` or emergency `Ctrl+C`.

**Architecture:** Preserve the existing typed `SkillDetail` load and shared modal/action geometry. Add a compact projection for the main detail pane, reuse the bounded text overlay for the loaded `SKILL.md` preview, and centralize quit handling before per-mode navigation while leaving text-entry modes intact.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, existing `internal/tui` render/input tests.

---

### Task 1: Bound Library details and expose loaded content on demand

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/modal.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/input_mouse.go`
- Test: `internal/tui/render_test.go`
- Test: `internal/tui/workflows_test.go`
- Test: `internal/tui/input_mouse_test.go`

- [ ] **Step 1: Rewrite the old scrolling regression and add failing compact-detail tests**

Replace `TestCompactSkillDetailWrapsLongContentInsideScrollableBody`, whose old contract requires the main pane to scroll to the final source line. Create a 220-line `SkillMD`, more than six usage scopes, more than ten files, and `SkillMDTruncated=true`. At widths 24/38/59/80/120 and representative heights 12/24, assert:

- the entire screen stays within terminal width/height;
- the loaded-detail body never exceeds the exact layout body budget after title and action bar;
- Usage and Files each consume no more than three wrapped display lines;
- source content consumes no more than six wrapped display lines;
- explicit omitted-location, omitted-file, omitted-source, and 64 KiB backend-truncation markers are visible;
- the final source line is absent from the main pane while fixed actions/footer remain visible.

- [ ] **Step 2: Run the compact-detail test and verify RED**

Run: `GOCACHE=/tmp/aikit-detail-red go test ./internal/tui -run 'TestLibraryDetail|TestCompactSkillDetail' -count=1`

Expected: FAIL because the current pane renders every file and all 220 content lines.

- [ ] **Step 3: Implement the compact projection**

Add a pure helper that takes `SkillDetail`, display width, and body budget. Allocate explicit sub-budgets of at most three wrapped rows for Usage, three for Files, and six for source content, then enforce the smaller remaining total body budget. Show explicit omission/backend-truncation markers inside those budgets. Use it only for the loaded Library main detail; other detail projections remain unchanged.

- [ ] **Step 4: Write failing full-preview keyboard/mouse tests**

After opening a loaded skill, assert the primary action is `View SKILL.md`; keyboard Enter and mouse click both open a scrollable text overlay titled `SKILL.md · <name>`. Assert PgDn and wheel reach the final loaded line, a 64 KiB truncation marker is shown when `SkillMDTruncated`, and Close restores the selected Library detail without a backend call.

- [ ] **Step 5: Run the full-preview tests and verify RED**

Run: `GOCACHE=/tmp/aikit-detail-red go test ./internal/tui -run 'TestLibraryDetail|TestSkillContent' -count=1`

Expected: FAIL because no `View SKILL.md` action or typed overlay title exists.

- [ ] **Step 6: Implement the on-demand preview overlay**

Generalize the existing bounded text-detail modal with a title field, preserve `Error details` for status/config callers, add `View SKILL.md` to loaded Library details, and dispatch keyboard/mouse through the same existing action handler. Append an explicit backend truncation marker when required.

- [ ] **Step 7: Run focused detail tests and verify GREEN**

Run: `GOCACHE=/tmp/aikit-detail-green go test ./internal/tui -run 'TestLibraryDetail|TestCompactSkillDetail|TestSkillContent|TestDetailScroll' -count=1`

Expected: PASS.

### Task 2: Make Ctrl+Q the only normal exit shortcut

**Files:**
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/render.go`
- Test: `internal/tui/operability_regression_test.go`
- Test: `internal/tui/foundation_blockers_test.go`
- Test: `internal/tui/model_test.go`

- [ ] **Step 1: Write the failing key-state matrix**

Table-drive List, Actions, Detail, Filter, Input, Confirm, More, ErrorDetail, Busy, and MutationBusy. Assert all resulting state, not only the command:

- Esc on root List is a strict no-op; Actions and Detail return to the collection without changing selection; Filter/Input cancel their drafts; Confirm/More/ErrorDetail close and restore their captured parent context; Busy ignores Esc without losing the in-flight read; MutationBusy ignores Esc and preserves both busy flags/write protocol state.
- page/non-text-modal `q` never quits and displays the `Ctrl+Q` hint, while Filter/Input append `q` to their draft/value.
- `Ctrl+Q` quits from every non-mutation state (including Busy after canceling read-only inventory) but leaves MutationBusy unchanged with a refusal status.
- `Ctrl+C` always produces emergency quit, including MutationBusy.

- [ ] **Step 2: Run the key tests and verify RED**

Run: `GOCACHE=/tmp/aikit-exit-red go test ./internal/tui -run 'Test.*Quit|Test.*Escape|TestExitKeyMatrix' -count=1`

Expected: FAIL because root Esc and page q currently quit, Ctrl+Q is unhandled, and MutationBusy blocks Ctrl+C.

- [ ] **Step 3: Implement centralized exit handling**

Handle `Ctrl+C` first as emergency quit. Refuse `Ctrl+Q` only during MutationBusy; otherwise cancel inventory and return `tea.Quit`. Preserve `q` as text in Filter/Input, but on every other state keep the model open and show `Press Ctrl+Q to quit`. Change root `uiCancel` to a no-op rather than quit.

- [ ] **Step 4: Update visible shortcut copy**

Replace every `q Quit` footer/help reference with `Ctrl+Q Quit`; describe `Ctrl+C` only as emergency quit. Keep `Esc` labels scoped to Back/Close/Cancel.

- [ ] **Step 5: Run focused exit tests and verify GREEN**

Run: `GOCACHE=/tmp/aikit-exit-green go test ./internal/tui -run 'Test.*Quit|Test.*Escape|TestExitKeyMatrix|TestFooter|TestHelp' -count=1`

Expected: PASS.

### Task 3: Real-data and release verification

**Files:**
- Modify only if a regression is found: `internal/tui/**`

- [ ] **Step 1: Run the complete TUI package**

Run: `GOCACHE=/tmp/aikit-tui-final go test ./internal/tui -count=1`

Expected: PASS.

- [ ] **Step 2: Run full tests, race detector, vet, and builds**

Run:

```bash
GOCACHE=/tmp/aikit-final go test ./... -count=1
GOCACHE=/tmp/aikit-race go test -race ./... -count=1
GOCACHE=/tmp/aikit-vet go vet ./...
GOCACHE=/tmp/aikit-build go build ./...
GOCACHE=/tmp/aikit-win GOOS=windows GOARCH=amd64 go build ./...
```

Expected: all commands exit 0.

- [ ] **Step 3: Run E2E and formatting checks**

Run: `GOCACHE=/tmp/aikit-e2e make test-e2e`

Run: `git diff --check`

Run: `test -z "$(gofmt -l internal/tui)"`

Expected: E2E PASS and both checks are silent.

- [ ] **Step 4: Read-only real-data TUI verification**

Run `make run` with the user's real HOME. Open the first 200+ line Library skill, verify the main details stay concise, open `View SKILL.md`, scroll to the end, close back to the same skill, confirm Esc/q stay in the application, and exit with Ctrl+Q. Do not confirm any mutation.
