# Workspace TUI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an English-first, mouse-and-keyboard workspace TUI with automatic read-only inventory, exact existing-skill adoption, progressive detail, and complete Library/Workspace/Preset/Migration/Status management.

**Architecture:** `internal/migrate` supplies a stable, exact-origin read-only inventory and adoption preview; `internal/app` supplies typed mutation previews and all writes; `internal/tui` owns responsive layout, hit testing, focus, overlays, and asynchronous command dispatch. Startup composes an offline `Snapshot` with an all-project dry-run inventory so opening the interface performs no network or durable writes.

**Tech Stack:** Go 1.25, Bubble Tea v1, Lip Gloss, Cobra integration, existing config/status/library/link/migrate services.

**Specification:** `docs/superpowers/specs/2026-08-13-tui-workspace-design.md`

---

## File map

```text
internal/app/types.go              frontend-safe preview and detail contracts
internal/app/preview.go            binding/remove/preset preview orchestration
internal/app/detail.go             safe library detail reads and resolved paths
internal/migrate/scan.go           all-project inventory and exact selectors
internal/migrate/preview.go        simulated import/adopt plans and state labels
internal/tui/model.go              top-level model and async message routing
internal/tui/rows.go               page projections and stable selection keys
internal/tui/layout.go             responsive rectangles and visible windows
internal/tui/input_keyboard.go     keyboard navigation
internal/tui/input_mouse.go        mouse hit testing mapped to shared actions
internal/tui/actions.go            shared UI state transitions and requests
internal/tui/commands.go           backend tea.Cmd boundaries
internal/tui/render.go             low-density two-pane rendering
internal/tui/styles.go             visual tokens
internal/tui/overlays.go           confirm/help/input/command/config overlays
internal/tui/launcher.go           default cell-motion mouse support
cmd/root.go                        default Overview launch and dependencies
README.md                           current TUI and migration workflow
scripts/test-e2e.sh                no-network startup/inventory smoke coverage
internal/web/**                    removed after replacement verification
```

## Execution order and hard contracts

Tasks are sequential unless a task explicitly says otherwise. Any task that changes a public frontend interface must update every production adapter/fake in the same task and run `go test ./...` with a writable task-specific `GOCACHE`; the tree must compile before the next task starts.

The following contracts are release gates, not later review suggestions:

- Inventory authorization uses a versioned SHA-256 key over length-prefixed origin and canonical absolute target bytes, but never trusts the key alone. Preview and mutation both revalidate key, origin, canonical target, root containment, object identity, and current content under the mutation lock.
- Inventory traversal reuses no-follow/containment primitives. Root symlinks, nested escaping symlinks, root replacement, preview/confirm target swaps, and selector aliases fail closed. An external symlink may be reported as an adopt candidate only when its resolved content passes the existing authorized discovery policy; it is never traversed as an inventory root.
- Startup inventory is streamed per authorized root with a generation ID. Refresh cancels the previous generation; stale events are discarded. Partial events merge by stable item key and never change the active identity, scroll anchor, or selections.
- A panic/counting update checker and Git runner are used in startup integration tests. Merely asserting `Offline:true` is insufficient proof of zero network calls.
- Unknown files, directories, external symlinks, malicious paths, and concurrently replaced objects in project cleanup roots must survive project edit/rebind/remove. Only exact, revalidated aikit-owned links may enter destructive plans.
- Pending recovery is a first-class app workflow. Every mutation is gated while pending recovery exists. Preview is read-only; Resume/Rollback are explicit, confirmed operations. Unsupported rollback states are reported rather than guessed.
- Every workflow action is registered once and exercised through a table-driven keyboard/mouse parity test. Workflow-specific buttons added after the generic mouse layer are not exempt.
- The existing modified `internal/web/frontend/tsconfig.app.tsbuildinfo` is user-owned. Record dirty paths and hashes before cleanup; do not delete or overwrite modified material without explicit user approval. All other deletions use explicit audited paths.

### Task 1: Exact all-project read-only inventory

**Files:**
- Modify: `internal/app/types.go`
- Modify: `internal/migrate/scan.go`
- Create: `internal/migrate/preview.go`
- Modify: `internal/migrate/scan_test.go`

- [ ] **Step 1: Write failing all-project and exact-selection tests**

Add tests with two registered projects and all supported global agents. Call `Scan` with `DryRun:true, AllProjects:true` and assert both projects' declared agent roots are included. Capture config bytes, library tree, recovery calls, and checkpoints before and after and assert they are unchanged.

Add two candidates with the same skill ID/name but different origins. Assert each `ScanItem.Key` is distinct and a request containing one `ScanSelector{Key, Origin, Target}` returns or adopts only that origin.

Add root-symlink, nested escape, external-link, canonical alias, root replacement, target swap after preview, and concurrent replacement cases. Assert inventory/adopt fails closed and never reads or changes the outside sentinel.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `go test ./internal/migrate -run 'TestScan(AllRegisteredProjects|SelectorUsesOriginAndTarget|DryRunIsStrictlyReadOnly)' -count=1`

Expected: compile failure for missing fields/types or behavior failure because only the current project is scanned.

- [ ] **Step 3: Implement the minimal inventory contract**

Add `ScanSelector{Key,Origin,Target}`, `ScanRequest.AllProjects/Selectors`, and a backend-generated inventory item containing `Key`, `Origin`, canonical `Target`, typed `config.Scope`, `Agent`, `Project`, discovered skill metadata/hash, matched library skill ID/hash, management state, diagnostic state/issues, and recommended `Action`. The action enum is `none`, `import`, `adopt`, `link-existing`, or `conflict`; the state enum covers every state in spec section 5.

Add table-driven backend classification tests for Managed, Unmanaged, Same content, Name conflict, Broken link, Drifted, Update available enrichment, Pending recovery, and Error. The TUI must not reconstruct these states from display strings.

Generate `Key` as SHA-256 over `aikit-inventory-v1\x00`, the big-endian length and bytes of origin, then the length and bytes of canonical absolute target. Keep target-only compatibility for CLI requests, but TUI writes must use selectors and require all selector fields to match. The mutation path authorizes origin and target again and never treats the hash as authorization. `AllProjects` enumerates every config project and only its declared agents. Sort roots by global agent registry order, then project name and agent order.

In dry-run adopt preview, classify managed links as link-existing, a new hash as import/adopt, identical library content as link-existing, and a same-name different-hash target as conflict. Simulate binding changes against a cloned config and call effective-scope validation without locks or writes.

- [ ] **Step 4: Verify GREEN and regression coverage**

Run: `go test ./internal/migrate ./internal/app -count=1`

Expected: PASS.

### Task 1B: Incremental cancellable inventory

**Files:**
- Modify: `internal/app/types.go`
- Modify: `internal/migrate/scan.go`
- Create: `internal/migrate/inventory.go`
- Create: `internal/migrate/inventory_test.go`

- [ ] **Step 1: Write failing stream tests**

Define tests for one event per authorized root, deterministic root IDs, partial root errors, cancellation of a blocked root, goroutine completion, and generation propagation. A canceled generation must emit no completion after cancellation.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/migrate -run 'TestInventoryStream' -count=1`

Expected: compile failure for the missing stream contract.

- [ ] **Step 3: Implement the minimal contract**

Add `InventoryRequest{Generation,AllProjects}` and `InventoryEvent{Generation,Root,Items,Issues,Completed,Total,Done}` plus `MigrationService.Inventory(context.Context, InventoryRequest) <-chan InventoryEvent`. Enumerate roots once, scan roots with a bounded worker count, close the channel on completion/cancellation, and keep stable sorting inside each event. Reuse the strict dry-run discovery path and perform no mutation/recovery/network operation.

- [ ] **Step 4: Verify GREEN and interface integration**

Run: `go test ./internal/migrate ./internal/app ./cmd ./internal/tui -count=1`

Expected: PASS and no leaking blocked worker.

### Task 2: Typed read-only previews and skill/config details

**Files:**
- Modify: `internal/app/types.go`
- Create: `internal/app/preview.go`
- Create: `internal/app/detail.go`
- Create: `internal/app/preview_test.go`
- Create: `internal/app/detail_test.go`

- [ ] **Step 1: Write failing preview contract tests**

Cover global skill enable, project common enable, project-agent disable, preset application, referenced-skill removal, preset removal, project creation, and project path edit. For each preview, snapshot config and filesystem and assert zero writes/recovery calls. Assert the preview contains affected scopes, exact link actions, references, conflicts, and whether force or confirmation is required.

Add detail tests asserting a skill ID resolves only inside the central library, returns metadata, enabled locations, `SKILL.md` content, and a stable file list. Add configuration detail tests for explicit `AIKIT_HOME` and default paths.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `go test ./internal/app -run 'TestPreview|TestSkillDetail|TestConfigurationDetail' -count=1`

Expected: compile failure for missing Service methods and result types.

- [ ] **Step 3: Add minimal frontend-safe contracts**

Extend `app.Service` with `PreviewBinding`, `PreviewRemove`, `PreviewPreset`, `SkillDetail`, and `Configuration`. `MutationPreview` contains a short title/summary, affected scopes, references, `link.Plan`, warnings, conflicts, and `RequiresForce/RequiresConfirmation`.

Implement previews by loading and cloning config, applying the requested logical change to the clone, validating it, and building dry-run plans. Never call `beforeMutation`, recovery, `Checkpoint`, Git, or a library mutation. Extend `PreviewProjectEdit` to accept `Project == ""` and simulate project creation.

Implement safe detail reads using existing library path-containment primitives. Truncate preview content by a documented byte limit while reporting truncation; never follow content paths outside the selected skill root.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/app -count=1`

Expected: PASS.

### Task 2B: Missing mutation, batch, compare, and recovery APIs

**Files:**
- Modify: `internal/app/types.go`
- Create: `internal/app/batch.go`
- Create: `internal/app/recovery.go`
- Create: `internal/app/compare.go`
- Create: `internal/app/batch_test.go`
- Create: `internal/app/recovery_test.go`
- Create: `internal/app/compare_test.go`
- Modify: all production adapters and frontend fakes affected by the interface freeze

- [ ] **Step 1: Write failing API tests**

Cover atomic batch enable/disable/remove/update; preset create/duplicate/rename/member edit/delete/apply; content Compare; and pending recovery Preview/Resume/Rollback. Assert conflicts abort a batch before durable change. Assert every ordinary mutation is rejected while pending recovery needs attention. Assert cancel paths call no mutation.

Recovery tests cover each supported pending kind and explicitly report rollback-unavailable states. Project cleanup tests include unknown directories/files, external symlinks, malicious paths, and concurrent replacements at old/new roots and assert all unknown content survives.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/app -run 'TestBatch|TestPresetMutation|TestCompare|TestRecovery|TestProjectUnknownContentSurvives' -count=1`

Expected: compile or behavior failure for missing APIs/gates.

- [ ] **Step 3: Implement app-owned mutations**

Add typed batch requests and results; preset mutation requests; compare results containing metadata/file/content diffs; and `PreviewRecovery`, `ResumeRecovery`, and `RollbackRecovery`. Application methods own locks, previews, validation, checkpoints, journal/recovery integration, and link execution. TUI and CLI never compose multi-call pseudo-transactions.

Exact Import/Adopt/Link-existing stays on `MigrationService` and is implemented by `internal/migrate` to preserve the dependency direction (`migrate` may use app types; app never imports migrate). Extend Task 1 ownership to `internal/migrate/preview.go` and its tests with typed exact-action Preview/Execute methods. Their preview result must list library additions, exact scope bindings, every replaced filesystem path, warnings/conflicts, and an item key for every eventual result.

Use narrower capability interfaces for optional detail/editor surfaces where possible. If `app.Service` changes, update every adapter/fake immediately and run the full repository tests before continuing.

- [ ] **Step 4: Verify GREEN and compiling tree**

Run: `go test ./... -count=1`

Expected: PASS.

### Task 3: Automatic offline startup and responsive TUI foundation

**Files:**
- Modify: `internal/tui/launcher.go`
- Modify: `internal/tui/model.go`
- Create: `internal/tui/layout.go`
- Create: `internal/tui/rows.go`
- Create: `internal/tui/styles.go`
- Modify: `internal/tui/commands.go`
- Modify: `internal/tui/model_test.go`
- Create: `internal/tui/layout_test.go`

- [ ] **Step 1: Write failing startup and layout tests**

Assert `Init` sequences `Snapshot(StatusRequest{Offline:true})` and the all-project `Inventory` stream. Assert the snapshot renders before the first inventory event, startup invokes neither update checker/Git runner nor mutation methods, partial root errors remain visible, and every incremental merge preserves the active item by identity, scroll anchor, and selections. Refresh must cancel the old generation; late old-generation messages must be ignored.

Test layout at widths 20, 38, 54, 55, 80, and 120 and heights 8, 12, 16, and 30. Rectangles must not overlap or escape the terminal. Narrow mode uses one pane with a breadcrumb; medium/wide modes use two panes. Visible-row calculations map absolute cursor indices correctly.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/tui -run 'TestStartup|TestLayout' -count=1`

Expected: failure because snapshot is online, scan is manual, and no shared layout exists.

- [ ] **Step 3: Implement startup orchestration and pure layout**

Make startup snapshot explicitly offline and launch inventory immediately after the snapshot message without blocking first render. Convert channel events to repeated Bubble Tea commands, retain the active generation, cancel prior refresh contexts, and reject stale messages. Represent inventory loading/partial/complete independently of mutation Busy.

Add Overview, Workspaces, Migration, and Configuration views plus explicit tab/list/detail/filter/overlay focus. Create pure `Rect` and `Layout` types used by render and hit testing. Split row projection from model/message routing. Selection remains keyed by backend inventory key or skill/scope composite key.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/tui -count=1`

Expected: PASS.

### Task 4: Low-density workspace rendering

**Files:**
- Replace: `internal/tui/views.go`
- Create: `internal/tui/render.go`
- Create: `internal/tui/overlays.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/views_test.go`
- Create: `internal/tui/render_test.go`

- [ ] **Step 1: Write failing rendering tests**

Use semantic assertions rather than full ANSI snapshots. Verify Overview attention summaries; Library rows with one primary state plus source/state filters and batch bar; Workspaces grouped as Global/Agents/Projects and projections for enabled/available/suppressed/conflicted/inherited/preset-provided; Preset member/application summaries; all inventory states (`Managed`, `Unmanaged`, `Same content`, `Name conflict`, `Broken link`, `Drifted`, `Update available`, `Pending recovery`, `Error`); Migration Import/Adopt/Link existing/Compare/Ignore actions; Status actionable issue groups; narrow breadcrumbs; mutually exclusive overlays; and English-only visible copy.

Assert heavy content (`SKILL.md`, file trees, hashes, full paths, and exact plans) is absent until its detail tab opens, no detail shows more than three primary actions, secondary actions live under More, and primary workflows remain within three deliberate interactions after the target row is visible.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/tui -run 'TestRender' -count=1`

Expected: behavior failure against the current dense table.

- [ ] **Step 3: Implement contextual rendering**

Render top tabs, collection list, detail pane, concise status line, and a context footer. Show no more than three detail actions; route secondary actions through More. Render search, batch bar, confirmation, help, configuration, command palette, and input overlays as mutually exclusive layers. Use terminal display width, not rune count, for clipping.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/tui -count=1`

Expected: PASS.

### Task 5: Mouse and keyboard parity

**Files:**
- Modify: `internal/tui/launcher.go`
- Create: `internal/tui/input_keyboard.go`
- Create: `internal/tui/input_mouse.go`
- Create: `internal/tui/actions.go`
- Replace: `internal/tui/keys.go`
- Create: `internal/tui/input_mouse_test.go`
- Modify: `internal/tui/model_test.go`

- [ ] **Step 1: Write failing hit-test and parity tests**

For tabs, visible rows, checkbox column, detail actions, confirm/cancel buttons, wheel scrolling, and breadcrumbs, send mouse messages at rectangle boundaries and compare the resulting model/request to the keyboard equivalent. Test scrolled lists, empty regions, overlays, Busy gating, cancellation, and repeated confirm clicks.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/tui -run 'TestMouse|TestKeyboardMouseParity' -count=1`

Expected: failure because launcher and model do not handle mouse messages.

- [ ] **Step 3: Implement shared actions and mouse support**

Enable `tea.WithMouseCellMotion()` by default. Translate mouse hit regions into the same pure action helpers used by keyboard input. Single-click selects a row; explicit buttons or Enter activate it. Wheel moves only the focused pane. Do not depend on hover, right-click, or double-click.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/tui -count=1`

Expected: PASS.

### Task 6A: Library and update workflows

**Files:**
- Modify: `internal/tui/commands.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/rows.go`
- Create: `internal/tui/workflow_library_test.go`

- [ ] **Step 1: Write failing workflow tests**

Cover add local/Git and candidate selection, state/source filters, multi-select batch enable/disable/update/remove, update/ref change, referenced remove, and SKILL.md/files/bindings/source/diagnostic detail. Referenced remove must require an explicit second Force Remove decision after the reference preview; the first confirmation may not imply force. Every action is entered in the shared action registry and table-driven keyboard/mouse parity matrix. Every workflow needs cancel-zero-write and one-submit-while-Busy coverage.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/tui -run 'TestLibraryWorkflow|TestLibraryActionParity' -count=1`

Expected: failure for missing overlays/actions.

- [ ] **Step 3: Implement workflows incrementally**

Use one reusable input/choice overlay state with context validation. Each write sequence is preview command, exact summary, confirmation, one mutation command, and snapshot/inventory refresh. Preserve partial results and structured issues rather than replacing them with a generic error. Batch actions call the app batch API once.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/tui ./internal/app ./internal/migrate -count=1`

Expected: PASS.

### Task 6B: Workspace and project workflows

**Files:**
- Modify: `internal/tui/commands.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/rows.go`
- Create: `internal/tui/workflow_workspace_test.go`

- [ ] **Step 1: Write RED tests** for Global/Agent/Project Common/Project Agent enable/disable, preset application, effective/suppressed/conflicted/inherited projections, sync preview, project create/edit/rebind/remove, and unknown-content preservation surfaced from app results. Include keyboard/mouse parity and cancel/Busy cases.
- [ ] **Step 2: Verify RED** with `go test ./internal/tui -run 'TestWorkspaceWorkflow|TestProjectWorkflow|TestWorkspaceActionParity' -count=1`.
- [ ] **Step 3: Implement minimal contextual workflows** using only typed app previews/mutations.
- [ ] **Step 4: Verify GREEN** with `go test ./internal/tui ./internal/app -count=1`.

### Task 6C: Preset workflows

**Files:** same focused TUI files plus `internal/tui/workflow_preset_test.go`.

- [ ] **Step 1: Write RED tests** for create, edit members, duplicate, rename, delete/reference protection, usage inspection with every applied scope, and apply target selection across Global/Agent/Project Common/Project Agent. Include keyboard/mouse parity, cancel, and Busy cases.
- [ ] **Step 2: Verify RED** with `go test ./internal/tui -run 'TestPresetWorkflow|TestPresetActionParity' -count=1`.
- [ ] **Step 3: Implement via the atomic app preset API.**
- [ ] **Step 4: Verify GREEN** with `go test ./internal/tui ./internal/app -count=1`.

### Task 6D: Migration, compare, ignore, and recovery workflows

**Files:** focused TUI files plus `internal/tui/workflow_migration_test.go`.

- [ ] **Step 1: Write RED tests** for exact-origin Import/Adopt/Link existing, Compare, session-only Ignore, legacy migrate preview, pending-recovery mutation gate, Review Recovery, Resume, safe Rollback, unsupported rollback, cancel, Busy, and keyboard/mouse parity. Confirmation assertions must include every library addition, exact binding, replaced path, and selected key; execution must retain a per-item result for success and failure.
- [ ] **Step 2: Verify RED** with `go test ./internal/tui -run 'TestMigrationWorkflow|TestRecoveryWorkflow|TestMigrationActionParity' -count=1`.
- [ ] **Step 3: Implement via exact selectors and recovery APIs.** Ignore is model-session state only and does not write config in this version.
- [ ] **Step 4: Verify GREEN** with `go test ./internal/tui ./internal/app ./internal/migrate -count=1`.

### Task 6E: Status and explicit update workflows

**Files:** focused TUI files plus `internal/tui/workflow_status_test.go`.

- [ ] **Step 1: Write RED tests** for issue grouping, Sync preview/apply, Retry, Compare, Copy Error, manual update check, offline cache display, partial errors, explicit ref, cancel, Busy, and keyboard/mouse parity. Update confirmation must show full current/remote identity and ref. Commit/update failure tests must assert the old library bytes and ledger identity remain unchanged. Explicit Refresh must call the update checker while never mutating config/library during the check phase.
- [ ] **Step 2: Verify RED** with `go test ./internal/tui -run 'TestStatusWorkflow|TestUpdateWorkflow|TestStatusActionParity' -count=1`.
- [ ] **Step 3: Implement using existing typed status/update/app calls.**
- [ ] **Step 4: Verify GREEN** with `go test ./internal/tui ./internal/updatecheck ./internal/app -count=1`.

### Task 6F: Configuration and command palette

**Files:**
- Modify: focused TUI files
- Create: `internal/tui/external.go`
- Create: `internal/tui/workflow_configuration_test.go`

- [ ] **Step 1: Write RED tests** for Show Configuration as an overlay/internal route (not a top-level tab), Copy Path, Validate, Reload, Edit in `$EDITOR`, command palette filtering, keyboard/mouse parity, and cancellation. Cover paths with spaces, missing editor, editor failure, and hostile editor strings.
- [ ] **Step 2: Verify RED** with `go test ./internal/tui -run 'TestConfigurationWorkflow|TestCommandPalette|TestConfigurationActionParity' -count=1`.
- [ ] **Step 3: Implement injected `Clipboard` and `EditorRunner` interfaces.** Execute the editor directly without a shell, parse only a supported executable-plus-args form, append the exact config path as one argument, and validate/reload after successful exit. If safe parsing is not possible, show the exact path/command without execution. Copy/reload/validate remain usable.
- [ ] **Step 4: Verify GREEN** with `go test ./internal/tui ./internal/app -count=1`.

### Task 7: Command integration and legacy cleanup

**Files:**
- Modify: `cmd/root.go`
- Modify: `cmd/root_test.go`
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-08-13-global-skills-manager-design.md`
- Modify: `docs/superpowers/plans/2026-08-13-global-skills-manager.md`
- Modify: `scripts/test-e2e.sh`
- Audit and conditionally modify: `go.mod`, `go.sum`, `Makefile`, `.gitignore`, CI/release workflows
- Delete with explicit path list generated by `rg --files internal/web`: unmodified `internal/web/**`
- Delete: empty `internal/asset/`, `internal/catalog/`, `internal/discovery/`, `internal/skill/`, `internal/source/`
- Delete: obsolete Catalog/Web UI documents and assets after reference audit

- [ ] **Step 1: Write failing integration tests**

Assert no-argument TTY launches Overview, command continuations enter the correct contextual overlay without losing source/id/ref, and startup performs no update network calls using panicking/counting update and Git seams. Extend local E2E with two projects plus global unmanaged skills and verify inventory keys/summaries.

- [ ] **Step 2: Verify RED**

Run: `go test ./cmd -run 'TestRootLaunchesOverview|TestTUIContinuationContext' -count=1`

Expected: failure against the current Library default/context mapping.

- [ ] **Step 3: Wire Overview and remove obsolete product files**

Update launcher context, README, and the original global-manager spec/plan so their TUI behavior, CLI+TUI runtime boundary, automatic local inventory, and completed legacy cleanup match the replacement. Before deletion, record `git status --short`, tracked paths, and hashes of every dirty path. The existing dirty `internal/web/frontend/tsconfig.app.tsbuildinfo` is not removed without explicit user direction. Audit `go.mod`, `go.sum`, Makefile, CI/release workflows, `.gitignore`, embed/static references, Gin/Vite dependencies, and docs with `rg`. Remove only explicit unmodified Web/frontend/build paths and empty legacy directories after replacement integration tests pass. Preserve current specs/plans and E2E coverage.

- [ ] **Step 4: Verify the replacement**

Run `rg` for obsolete live references and compare unrelated dirty-file hashes, then run `go test ./... -count=1`, `go test -race ./... -count=1`, `go vet ./...`, `go build ./...`, `GOOS=windows GOARCH=amd64 go build ./...`, `make test-e2e`, and `git diff --check` with a writable task-specific `GOCACHE`.

Expected: no live obsolete references; every command exits 0.

### Task 8: Independent acceptance review

**Files:** Review only initially; fixes go to owning files above.

- [ ] **Step 1: Review every design acceptance criterion**

Treat startup network/write behavior, exact-origin authorization, cancellation, repeated-click mutation, and unknown-content deletion as release blockers.

- [ ] **Step 2: Review code quality and interaction behavior**

Inspect model size, shared keyboard/mouse actions, rectangles, display widths, async cancellation, partial errors, and frontend/business separation. Fix all Critical and Important findings and re-review.

Run the focused inventory cancellation/leak test with a canceled context and blocked fake root. Assert the worker goroutine exits and no late message from the canceled generation changes a refreshed model.

- [ ] **Step 3: Perform fresh final verification**

Repeat the full Task 7 verification from a fresh test cache where practical. Record exact commands/results and never claim completion from agent reports or earlier runs.
