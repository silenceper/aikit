# Skills Source Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add safe GitHub and skills.sh source resolution plus explicit remote discovery and candidate selection before writing skills into the central Library.

**Architecture:** A pure library source resolver converts supported user input into a clone source and optional candidate selection. Offline preview only resolves; an explicitly network-enabled temporary discovery returns candidates and identity evidence; final Add revalidates that evidence before checkpoint/commit. TUI reuses its existing add-select checklist between the network and mutation confirmations.

**Tech Stack:** Go, Cobra, Bubble Tea, existing library Git Runner and config transaction layer.

---

### Task 1: Resolve GitHub and skills.sh sources

**Files:**
- Create: `internal/library/add_source.go`
- Create: `internal/library/add_source_test.go`
- Modify: `internal/library/source_kind.go`

- [ ] Write failing table tests for local paths, GitHub shorthand, full Git transports, exact skills.sh pages/repositories, and unsafe skills.sh URLs.
- [ ] Run `go test ./internal/library -run 'TestResolveAddSource' -count=1` and verify RED.
- [ ] Implement the pure resolver and make classification consume it without changing local-path precedence.
- [ ] Run the focused tests and `go test ./internal/library -count=1` and verify GREEN.

### Task 2: Add temporary network discovery

**Files:**
- Modify: `internal/library/service.go`
- Modify: `internal/library/service_test.go`

- [ ] Write failing tests for default/ref checkout, candidate discovery, cleanup, no persistent cache/library writes, and cancellation/error cleanup.
- [ ] Run `go test ./internal/library -run 'TestPreviewGit' -count=1` and verify RED.
- [ ] Implement a temp-only Git discovery method using the existing Runner and discovery/containment validation.
- [ ] Run focused and package tests and verify GREEN.

### Task 3: Freeze typed preview and revalidation contracts

**Files:**
- Modify: `internal/app/types.go`
- Modify: `internal/app/library_adapter.go`
- Modify: `internal/app/library.go`
- Modify: `internal/app/preview_test.go`
- Modify: `internal/app/app_test.go`
- Modify: `cmd/root.go`
- Modify: `cmd/root_test.go`

- [ ] Write failing tests for offline resolution, explicit network discovery, skills.sh suggested selection, and resolved/hash mismatch rejection before mutation.
- [ ] Run focused app/cmd tests and verify RED.
- [ ] Add the request/result identity fields, connect temporary discovery, and revalidate prepared entries.
- [ ] Update service adapters/fakes and run `go test ./internal/app ./cmd -count=1` GREEN.

### Task 4: Implement the TUI two-stage flow

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/commands.go`
- Modify: `internal/tui/input_mouse.go`
- Modify: `internal/tui/rows.go`
- Modify: `internal/tui/workflows_test.go`
- Modify: `internal/tui/model_test.go`

- [ ] Write failing keyboard/mouse tests for network confirmation -> discovery -> selection -> final confirmation -> one Add call.
- [ ] Add cancellation, zero-selection, skills.sh preselection, multi-select, busy, and narrow-screen tests; verify RED.
- [ ] Add a distinct discovery confirmation action and populate `ModeAddSelect` from the network result.
- [ ] Build final `AddRequest` identity evidence from selected candidates and verify focused/package GREEN.

### Task 5: CLI compatibility and documentation

**Files:**
- Modify: `cmd/commands.go`
- Modify: `cmd/root_test.go`
- Modify: `README.md`

- [ ] Write failing tests that exact skills.sh pages add only their page skill when `--skill` is absent and explicit `--skill` overrides it.
- [ ] Implement CLI source resolution/selection propagation without adding marketplace/API dependencies.
- [ ] Document TUI and CLI examples for GitHub, skills.sh, local paths, and multi-skill repositories.
- [ ] Run focused cmd tests GREEN.

### Task 6: Verification

- [ ] Run `go test ./... -count=1`.
- [ ] Run `go test -race ./... -count=1`.
- [ ] Run `go vet ./...`.
- [ ] Run native and Windows `go build ./...`.
- [ ] Run `make test-e2e`.
- [ ] Run `git diff --check` and ensure `gofmt -l` is empty for changed Go files.
- [ ] Confirm the pre-existing Preset mouse fix remains covered and unchanged in behavior.

