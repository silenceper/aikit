# Path-first Project Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace encoded TUI project/scope inputs with path-first project registration, structured project management, and workspace-local skill/preset operations that meet the approved aikit TUI requirements and the relevant workspace ergonomics of skills-manager.

**Architecture:** `internal/app` owns read-only project registration discovery, path validation, derived defaults, exact previews, and every mutation. `internal/tui` owns only typed picker state, navigation, rendering, and translation into app requests. Existing `EditProject`, `PreviewProjectEdit`, `PreviewBinding`, `PreviewPresetMutation`, `Batch`, and recovery contracts remain authoritative; the TUI never edits config or links directly.

**Tech Stack:** Go 1.25 baseline (verified with the installed Go toolchain), Bubble Tea, Lip Gloss, existing `internal/app`, `internal/agent`, `internal/link`, `pkg/config`, table-driven Go tests.

**Specifications:** `docs/superpowers/specs/2026-08-17-path-first-project-management-design.md` and the already approved `docs/superpowers/specs/2026-08-13-tui-workspace-design.md`.

**Benchmark boundary:** Compare the current product with the workspace workflows documented by the official [xingkongliang/skills-manager README](https://github.com/xingkongliang/skills-manager): central library, per-agent global workspace, project workspace, preset activation, add-skills selection, batch actions, live state, source preview, and explicit updates. Marketplace, tags, custom tools, activity logs, archive sources, backup, and multi-machine sync remain outside aikit's approved first-version scope.

---

## File map

- Create `internal/app/project_registration.go`: shared project-directory validation, safe agent detection, and typed registration preview.
- Create `internal/app/project_identity_unix.go`, `project_identity_windows.go`, and `project_identity_other.go`: strong opaque directory identity and reparse handling.
- Create `internal/app/project_registration_test.go`: read-only, detection, duplicate, mutation-boundary, and platform-neutral safety tests.
- Modify `internal/app/types.go`: typed registration request/preview/name-issue contract and `Service` method.
- Modify `internal/app/projects.go`: reuse shared directory validation at creation/path-change mutation boundaries.
- Modify `internal/app/batch.go` and create `internal/app/batch_preview_test.go`: one app-owned pure binding-batch preview shared with mutation preflight.
- Create `internal/tui/project_workflow.go`: project registration, rename/path, agent checklist, preset target, and post-refresh routing state.
- Create `internal/tui/scope_picker.go`: reusable typed binding/preset scope choices; no encoded scope strings.
- Create `internal/tui/workspace_projection.go`: direct/preset/common/global state projection for workspace skill rows.
- Modify `internal/tui/model.go`, `commands.go`, `actions.go`, `rows.go`, `render.go`, `overlays.go`, `input_keyboard.go`, and `input_mouse.go`: connect typed workflows to the existing shared focus/action/hit geometry.
- Modify `cmd/root.go` and test fakes only to satisfy the extended `app.Service` contract; CLI command syntax remains unchanged.
- Create `internal/tui/project_registration_test.go`, `project_management_test.go`, `scope_picker_test.go`, `workspace_projection_test.go`, and `skills_manager_parity_test.go`.
- Create `docs/tui-skills-manager-parity.md`: evidence-based final capability matrix, exclusions, and interaction counts.

### Task 1: Record the authoritative parity matrix

**Files:**
- Create: `docs/tui-skills-manager-parity.md`
- Test: `internal/tui/skills_manager_parity_test.go`

- [ ] **Step 1: Write the failing discoverability test**

Add a table-driven test that begins from each normal TUI page and uses only visible actions plus keyboard/mouse events. Require discoverable routes for:

```go
[]string{
    "Library/Add source",
    "Library/Batch",
    "Workspaces/Global/Skills",
    "Workspaces/Global/Apply preset",
    "Workspaces/Global agent/Skills",
    "Workspaces/Global agent/Apply preset",
    "Workspaces/Project/Create from path",
    "Workspaces/Project/Common skills",
    "Workspaces/Project/Agent skills",
    "Workspaces/Project/Manage agents",
    "Workspaces/Project/Apply preset",
    "Presets/Create",
    "Migration/Adopt",
    "Status/Recovery",
}
```

Assert no required route opens an input prompt containing `|`, `agent:name`, `project:name`, or `project-agent:`.

- [ ] **Step 2: Run the test and verify RED**

Run:

```bash
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp \
  go test ./internal/tui -run TestSkillsManagerParityRoutesAreVisibleAndStructured -count=1 -v
```

Expected: FAIL on project create/edit and encoded preset/batch scope inputs.

- [ ] **Step 3: Write the initial audit document**

Record current evidence as `Meets`, `Partial`, `Missing`, or `Excluded`. Treat aikit's approved specification as the requirement source and skills-manager only as the interaction benchmark. Include the explicit exclusions above so parity does not silently expand into network marketplace, backup, or custom-tool work.

- [ ] **Step 4: Commit the RED test and audit baseline**

```bash
git add docs/tui-skills-manager-parity.md internal/tui/skills_manager_parity_test.go
git commit -m "test: audit skills manager TUI parity"
```

### Task 2: Add a typed read-only project registration preview

**Files:**
- Create: `internal/app/project_registration.go`
- Create: `internal/app/project_registration_test.go`
- Modify: `internal/app/types.go`
- Modify: `internal/app/projects.go`
- Modify: `cmd/root.go`
- Modify: `cmd/root_test.go`
- Modify: `internal/tui/model_test.go`

- [ ] **Step 1: Write failing app tests**

Cover:

```go
func TestPreviewProjectRegistrationDefaultsNameAndDetectsAgents(t *testing.T)
func TestPreviewProjectRegistrationNoAgentsDoesNotSelectAll(t *testing.T)
func TestPreviewProjectRegistrationReturnsTypedNameConflict(t *testing.T)
func TestPreviewProjectRegistrationIsReadOnly(t *testing.T)
func TestPreviewProjectRegistrationRejectsMissingFileAndUnsafeAgentEntry(t *testing.T)
func TestEditProjectRevalidatesDirectoryAtMutationBoundary(t *testing.T)
```

The read-only test snapshots config bytes and the complete directory tree, supplies counting recovery/execute seams, invokes preview, and asserts byte-for-byte equality plus zero mutation calls.

- [ ] **Step 2: Run tests and verify RED**

```bash
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp \
  go test ./internal/app -run 'TestPreviewProjectRegistration|TestEditProjectRevalidatesDirectory' -count=1 -v
```

Expected: compile failure because the typed API does not exist.

- [ ] **Step 3: Add the typed contract**

Add to `internal/app/types.go`:

```go
type ProjectNameIssue string

const (
    ProjectNameInvalid   ProjectNameIssue = "invalid"
    ProjectNameDuplicate ProjectNameIssue = "duplicate"
)

type ProjectRegistrationRequest struct {
    Path string
    Name string
}

type ProjectRegistrationPreview struct {
    Name      string
    Path      string
    PathIdentity string
    Agents    []string
    Preview   ProjectEditPreview
    Warnings  []string
    NeedsName bool
    NameIssue ProjectNameIssue
}
```

Extend `ProjectEditRequest` with `ExpectedPathIdentity string` and
`ProjectEditPreview` with `PathIdentity string`. The field is an opaque
confirmation token; the TUI must copy it from preview and must never construct
or parse it.

Extend `Service` with:

```go
PreviewProjectRegistration(context.Context, ProjectRegistrationRequest) (ProjectRegistrationPreview, error)
```

- [ ] **Step 4: Implement shared directory validation and detection**

In `project_registration.go` and the platform identity files:

- resolve an absolute canonical path and require a readable directory;
- open it with `os.OpenRoot`;
- use `Root.Lstat` on every component of each `agent.All()` `ProjectSkillDir` relative path;
- select only a real final directory whose inspected components are not symlinks/reparse points;
- append a warning for non-directory, unsafe, or unreadable entries;
- derive basename unless `request.Name` is non-empty;
- load config only, detect duplicate canonical path, and return typed `NeedsName` for invalid/duplicate names;
- call a shared clone-based project preview helper only when the name is usable;
- return device+inode identity on Darwin/Linux and volume serial+file index on Windows;
- check `FILE_ATTRIBUTE_REPARSE_POINT` explicitly on Windows; and
- fail closed on unsupported platforms.

Update `EditProject` and `PreviewProjectEdit` to call the same existing-directory validator for new projects and path changes immediately before building/checkpointing their config change.
When `ExpectedPathIdentity` is non-empty, `EditProject` compares it with the
fresh identity before any mutation and rejects replacement. Add create and path
change tests that replace or retarget the previewed directory before confirm.

- [ ] **Step 5: Update adapters/fakes and verify GREEN**

Implement the new method in the real app, cmd error adapter, and test fakes. Run:

```bash
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp \
  go test ./internal/app ./cmd ./internal/tui -run 'TestPreviewProjectRegistration|TestEditProjectRevalidatesDirectory|TestRoot' -count=1 -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/project_registration.go internal/app/project_identity_*.go internal/app/project_registration_test.go internal/app/types.go internal/app/projects.go cmd/root.go cmd/root_test.go internal/tui/model_test.go
git commit -m "feat: add project registration preview"
```

### Task 3: Replace project creation with a path-only flow

**Files:**
- Create: `internal/tui/project_workflow.go`
- Create: `internal/tui/project_registration_test.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/commands.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/input_mouse.go`
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/workspace_project_test.go`

- [ ] **Step 1: Write failing keyboard/mouse path-only tests**

Test both keyboard and mouse from an empty Projects page:

1. `Create project` opens `Project directory`.
2. Entering `/work/aikit` sends exactly `ProjectRegistrationRequest{Path:"/work/aikit"}`.
3. The returned confirmation displays name, canonical path, detected agents, warnings, cleanup/next actions, and retains the opaque path identity without rendering or parsing it.
4. Escape makes zero `EditProject` calls.
5. Confirm sends exactly one `ProjectEditRequest{Name:"aikit", Path:"/work/aikit", AddAgents:..., ExpectedPathIdentity:preview.PathIdentity, Confirmed:true}`.
6. A successful snapshot refresh opens `Scope{Project:"aikit", Level:"project-targets"}`.
7. `NeedsName` opens a separate `Project name` input and rebuilds preview with `Name` set.
8. No visible prompt or error contains `pipe-separated`.

- [ ] **Step 2: Run and verify RED**

```bash
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp \
  go test ./internal/tui -run 'TestProjectRegistration' -count=1 -v
```

Expected: FAIL because creation still parses three fields.

- [ ] **Step 3: Implement the typed registration state machine**

Add explicit pending registration state to `Model`. `inputProjectCreate` accepts only a path and invokes `projectRegistrationPreviewCmd`. A typed preview message either:

- opens the exact confirmation;
- opens a separate name input when `NeedsName`; or
- preserves the input and shows the returned error.

Confirmation calls `EditProject` once. Record a pending post-refresh destination and restore by stable project identity after snapshot completion.

Delete the create branch of `parseProjectEditInput` and all `name|path|agents` copy/tests.

- [ ] **Step 4: Verify GREEN and regressions**

```bash
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp \
  go test ./internal/tui -run 'TestProjectRegistration|TestMouseConfirmCancelBusyGate|TestNarrow' -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/project_workflow.go internal/tui/project_registration_test.go internal/tui/model.go internal/tui/commands.go internal/tui/actions.go internal/tui/input_mouse.go internal/tui/render.go internal/tui/overlays.go internal/tui/workspace_project_test.go
git commit -m "feat(tui): add path-first project registration"
```

### Task 4: Add structured project management

**Files:**
- Create: `internal/tui/project_management_test.go`
- Modify: `internal/tui/project_workflow.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/rows.go`
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/input_mouse.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/workspace_project_test.go`

- [ ] **Step 1: Write failing project-detail tests**

Require keyboard/mouse equivalent, discoverable routes for:

- rename using one name input;
- change path using one path input and exact cleanup/next preview;
- keyboard and mouse copying `ProjectEditPreview.PathIdentity` into the exact
  confirmed `ProjectEditRequest.ExpectedPathIdentity` for every path change;
- manage agents as five checkbox rows with detected/current state;
- saving agents as one exact add/remove diff;
- remove project using the existing typed preview;
- cancellation at input, checklist, and confirmation causing zero mutations;
- Common and each declared Agent opening the existing skill-binding collection;
- empty projects showing `No agents configured` and a visible `Manage agents` action.

- [ ] **Step 2: Run and verify RED**

```bash
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp \
  go test ./internal/tui -run 'TestProjectRename|TestProjectPath|TestProjectAgent|TestEmptyProject' -count=1 -v
```

- [ ] **Step 3: Implement project management states**

Add a project management More menu for both the selected project row and its target page. Replace `inputProjectEdit` with separate `inputProjectRename` and `inputProjectPath`. Add a checklist mode whose rows are `agent.Names()` and whose Save action computes:

```go
ProjectEditRequest{
    Project: selected.Name,
    AddAgents: difference(selectedAgents, selected.Agents),
    RemoveAgents: difference(selected.Agents, selectedAgents),
}
```

Every operation first calls `PreviewProjectEdit`, displays exact Cleanup/Next diagnostics, then sends one confirmed `EditProject` request.

- [ ] **Step 4: Preserve stable scope after changes**

After rename, route to the renamed project. After agent removal, route to `Common` if the selected agent disappeared. After project removal, route to the project collection. Restore cursor/scroll by stable key.

- [ ] **Step 5: Verify GREEN and commit**

```bash
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp \
  go test ./internal/tui ./internal/app -run 'TestProject|TestEmptyProject' -count=1 -v
git add internal/tui
git commit -m "feat(tui): add structured project management"
```

### Task 5: Replace encoded scope inputs with a reusable picker

**Files:**
- Create: `internal/tui/scope_picker.go`
- Create: `internal/tui/scope_picker_test.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/rows.go`
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/input_mouse.go`
- Modify: `internal/tui/preset_task6c_test.go`
- Modify: `internal/tui/library_task6a_test.go`

- [ ] **Step 1: Write failing structured-picker tests**

Build choices from the snapshot in deterministic order:

```text
All agents
Global / cursor
Global / claude-code
...
Project / aikit / Common
Project / aikit / codex
...
```

Require search, arrows/j/k, row click, Enter/click selection, Escape zero-write,
exact request parity, narrow mouse reachability, and no colon mini-language. For
workspace-local preset apply, first choose a preset from a typed preset picker;
only then choose or confirm its exact scope. Cancelling either picker returns
to the prior workspace with zero calls.

- [ ] **Step 2: Run and verify RED**

```bash
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp \
  go test ./internal/tui -run 'TestStructuredScopePicker' -count=1 -v
```

- [ ] **Step 3: Implement one typed picker**

Represent each choice with a label plus one or more exact
`app.BindingRequest` values. `All agents` expands to one binding per supported
agent and is previewed/executed atomically through `PreviewBatch`/`Batch`; the
TUI never invents an implicit config-wide binding. Use the picker for:

- Library batch enable/disable scope choice;
- Presets page Apply target choice;
- selected project `Apply preset`, prefiltered to its Common and declared agents;
- selected global Agent `Apply preset`, preselected to that agent.
- the distinct Workspaces `Global` page, where users can choose all or an exact
  subset of agents rather than editing one hidden default.

The project/global workspace entry first opens a preset picker populated from
`Snapshot.Config.Presets`. Selecting one stores its name, then opens the typed
scope picker (or directly previews the already exact global-agent scope).
Presets-page Apply already has a selected preset and starts at the scope picker.

Delete `parsePresetApplyTarget`, `parseBatchBindingScope`, `inputPresetApply`, and `inputBatchScope` once all call sites use the picker.

- [ ] **Step 4: Verify GREEN and commit**

```bash
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp \
  go test ./internal/tui -run 'TestStructuredScopePicker|TestPreset|TestBatchEnableDisable' -count=1 -v
git add internal/tui
git commit -m "feat(tui): add structured workspace target picker"
```

### Task 6: Make workspace skill state and batch actions truthful

**Files:**
- Modify: `internal/app/types.go`
- Modify: `internal/app/batch.go`
- Create: `internal/app/batch_preview_test.go`
- Modify: `cmd/root.go`
- Modify: `cmd/root_test.go`
- Create: `internal/tui/workspace_projection.go`
- Create: `internal/tui/workspace_projection_test.go`
- Modify: `internal/tui/rows.go`
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/render.go`
- Modify: `internal/tui/input_keyboard.go`
- Modify: `internal/tui/input_mouse.go`

- [ ] **Step 1: Write failing state-projection tests**

For All Agents Global, Global Agent, Project Common, and Project Agent scopes,
cover every source. The All Agents page projects every library skill with an
exact `enabled/total agents` count and expands to per-agent ownership detail:

```go
const (
    WorkspaceAvailable = "Available"
    WorkspaceDirect = "Direct"
    WorkspacePreset = "Preset: <name>"
    WorkspaceCommon = "Project common"
    WorkspaceGlobal = "Global inherited"
    WorkspaceConflict = "Conflict"
)
```

Assert preset-only/inherited rows do not advertise a misleading one-click
`Disable` that cannot remove the effective skill. Their detail must identify
the owning preset/scope and expose the relevant structured management route.

- [ ] **Step 2: Write failing app-owned batch preview tests**

Add `Service.PreviewBatch(context.Context, BatchRequest) (BatchPreview, error)`
and first write RED tests for every exposed operation: Enable, Disable, Remove,
and Update. Prove each request is validated as one unit without mutation lock,
recovery, checkpoint, library preparation, Git, or writes. Define:

```go
type BatchPreview struct {
    MutationPreview
    Items  []BatchItemResult
    Issues []OperationIssue
}
```

For Enable/Disable, apply all binding changes to one cloned config, validate
effective scopes together, and build one exact combined link plan. For Remove,
resolve every selected skill, aggregate references, enforce the first/second
force decision, simulate pruning on one clone, and build the exact cleanup
plan. For Update, validate every selected full ID and complete Expected token
against the same snapshot and return exact current/ref/remote item summaries;
it performs no remote resolution or library preparation.

Each mutation path and its preview must reuse the same operation-specific pure
preflight helper before the mutation adds journals or prepares library content,
so validation cannot diverge between confirmation and execution. Update the
real app, cmd adapter, and every TUI/cmd fake with the new service method.

- [ ] **Step 3: Write failing workspace batch TUI tests**

Require a visible `Add skills`/`Select skills` action within Agent, Project
Common, and Project Agent pages. The picker shows the Library with search and
multi-selection, previews one atomic `Batch` request for the current exact
scope through `PreviewBatch`, cancels with zero writes, and prevents duplicate
submit while busy. Table-drive keyboard and mouse through the same selection,
preview, confirm, and cancel requests at wide and 38-column layouts; assert all
visible buttons and checkboxes remain mouse reachable.

Also require a functional Workspaces `Global` page. It lists every library
skill with an exact enabled-agent count, opens per-agent detail, supports a
library multi-select plus agent target checklist, and applies Presets through
the same exact multi-agent batch preview. It must not silently select all
agents; `All agents` is a visible deliberate choice.

- [ ] **Step 4: Run and verify RED**

```bash
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp \
  go test ./internal/app ./internal/tui -run 'TestPreviewBatch|TestWorkspaceProjection|TestWorkspaceBatch' -count=1 -v
```

- [ ] **Step 5: Implement app-owned preview, projection, and batch selection**

Keep projection pure and presentation-only over `Snapshot.Config`; use full
skill IDs and preset expansion. Mutation still goes through
`PreviewBatch`/`Batch`; replace `batchBindingPreviewCmd`,
`batchRemovePreviewCmd`, and local Update confirmation summaries with one
`batchPreviewCmd`. The TUI must not compose per-binding/remove previews or
operation plans.
Reuse the existing selection/filter machinery, but
key selections by scope plus full skill ID. Preserve the current fast single
direct toggle only when the row is directly toggleable.

- [ ] **Step 6: Add mixed-provenance cases**

Cover direct+Preset, direct+Common, direct+Global, Preset+Common, and
Preset+Global combinations. The detail must list every owner while action
eligibility remains based on whether the current exact binding can change the
effective result.

- [ ] **Step 7: Verify GREEN and commit**

```bash
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp \
  go test ./internal/tui ./internal/app -run 'TestWorkspace' -count=1 -v
git add internal/app/types.go internal/app/batch.go internal/app/batch_preview_test.go internal/tui cmd/root.go cmd/root_test.go
git commit -m "feat(tui): expose workspace skill sources and batch actions"
```

### Task 7: Close the parity audit and verify the product

**Files:**
- Modify: `docs/tui-skills-manager-parity.md`
- Modify: `README.md`
- Test: `internal/tui/skills_manager_parity_test.go`

- [ ] **Step 1: Re-run the full discoverability matrix**

```bash
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp \
  go test ./internal/tui -run 'TestSkillsManagerParityRoutesAreVisibleAndStructured' -count=1 -v
```

Expected: PASS, with no encoded project/scope prompts.

- [ ] **Step 2: Update the audit and README**

Mark each in-scope row with the exact TUI route and authoritative test. Keep
excluded external features visibly excluded. Document the path-only creation
flow and project Common/per-agent/preset management.

- [ ] **Step 3: Run focused verification**

```bash
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp \
  go test ./internal/app ./internal/tui ./internal/migrate ./cmd -count=1
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp \
  go test -race ./internal/app ./internal/tui ./internal/migrate ./cmd -count=1
```

Expected: PASS.

- [ ] **Step 4: Run complete verification**

```bash
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp go test ./... -count=1
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp go test -race ./... -count=1
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp go vet ./...
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp go build ./...
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp GOOS=windows GOARCH=amd64 go build ./...
GOCACHE=/tmp/aikit-project-gocache GOTMPDIR=/tmp/aikit-project-gotmp make test-e2e
gofmt -l internal/app internal/tui cmd
git diff --check
```

Expected: all exit 0; `gofmt -l` prints nothing.

- [ ] **Step 5: Perform final interaction review**

Review the TUI at 120, 80, 59, 38, and 24 columns in default and `NO_COLOR`
modes. Verify every project/preset/scope action with keyboard and mouse, exact
preview content, cancellation, partial result display, and stable return
context. Treat any unreachable action, misleading state, or raw mini-language
as a release blocker.

- [ ] **Step 6: Commit final docs/tests**

```bash
git add docs/tui-skills-manager-parity.md README.md internal/tui/skills_manager_parity_test.go
git commit -m "docs: document TUI workspace parity"
```
