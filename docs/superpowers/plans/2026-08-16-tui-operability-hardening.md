# TUI Operability Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every visible TUI action valid, cancellation reversible, identity clear, and narrow-screen previews safe.

**Architecture:** Keep Bubble Tea and the existing application-service boundaries. Add presentation-only destination metadata, modal return state, filter drafts, contextual action selection, and responsive rendering helpers inside `internal/tui`.

**Tech Stack:** Go 1.25, Bubble Tea, Lip Gloss, table-driven Go tests

---

### Task 1: Navigation and modal state restoration

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/modal.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/rows.go`
- Test: `internal/tui/operability_regression_test.go`

- [x] Write failing tests for Overview destinations and migration-confirm cancellation.
- [x] Run the focused tests and confirm the expected failures.
- [x] Add typed attention destinations and saved confirmation parent/focus state.
- [x] Run the focused tests and confirm they pass.

### Task 2: Search transaction and visible collection state

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/render.go`
- Test: `internal/tui/operability_regression_test.go`

- [x] Write failing tests for draft/apply/cancel, visible query, and result count.
- [x] Run the focused tests and confirm the expected failures.
- [x] Implement filter draft state and collection-heading feedback.
- [x] Run the focused tests and confirm they pass.

### Task 3: Contextual actions and truthful help

**Files:**
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/input_mouse.go`
- Modify: `internal/tui/overlays.go`
- Test: `internal/tui/operability_regression_test.go`
- Test: `internal/tui/action_focus_test.go`

- [x] Write failing tests for invalid action removal and q/Esc behavior.
- [x] Run the focused tests and confirm the expected failures.
- [x] Derive primary/More actions from row capabilities and update help copy.
- [x] Run keyboard/mouse parity tests and confirm they pass.

### Task 4: Identity, responsive hierarchy, and exact previews

**Files:**
- Modify: `internal/tui/rows.go`
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/styles.go`
- Test: `internal/tui/render_test.go`
- Test: `internal/tui/operability_regression_test.go`

- [x] Write failing tests for duplicate origins, broken-link labels, narrow metrics, range indicators, and wrapped paths.
- [x] Run the focused tests and confirm the expected failures.
- [x] Implement the minimal responsive rendering and identity changes.
- [x] Run render matrices and `NO_COLOR` fixtures.

### Task 5: Configuration copy and final verification

**Files:**
- Modify: `internal/tui/render.go`
- Test: `internal/tui/configuration_test.go`

- [x] Add a failing test for the truthful `Show paths` label and read-only help.
- [x] Update configuration copy without adding clipboard or editor authority.
- [x] Run `go test -count=1 -race ./internal/tui`.
- [x] Run `go test -count=1 -race ./...`, `go vet ./...`, `make test-e2e`, cross-platform builds, and `git diff --check`.
- [x] Perform fresh 120x30, 80x24, 38x12, and 24x8 manual TUI walkthroughs.
