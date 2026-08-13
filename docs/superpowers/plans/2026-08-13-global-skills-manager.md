# Global Skills Manager Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the legacy project-copy/catalog product with a single-machine Skills manager backed by one global YAML ledger, a central library, symlink reconciliation, CLI automation, and a k9s-style TUI.

**Architecture:** `pkg/config` owns durable state and locking; focused internal packages own agent paths, library content, effective scope planning, reconciliation/recovery, status, and update detection. `internal/app` is the only orchestration layer used by Cobra and Bubble Tea. No CLI or TUI package writes config or IDE directories directly.

**Tech Stack:** Go 1.25, Cobra, yaml.v3, Bubble Tea v1, Bubbles, Lip Gloss, standard-library filesystem/process APIs, Git CLI through an injectable runner.

**Specification:** `docs/superpowers/specs/2026-08-13-global-skills-manager-design.md`

---

## Execution waves and ownership

- Wave 0 is sequential: Task 1 establishes shared types and interfaces.
- Wave 1 is parallel: Tasks 2, 3, and 4 own disjoint packages.
- Wave 2 starts after Wave 1: Task 5 establishes the application facade.
- Wave 3 is parallel: Tasks 6, 7, and 8 own migration/scan, CLI, and TUI respectively.
- Wave 4 is sequential: Tasks 9 and 10 remove legacy code, integrate, review, and verify.
- Agents must not edit `internal/web/frontend/tsconfig.app.tsbuildinfo`; it contains a pre-existing user change.
- Each implementation task gets a spec-compliance review followed by a code-quality review before its wave is accepted.

## Target file map

```text
pkg/config/                 schema, validation, atomic store, lock
internal/agent/             canonical agent registry and path resolution only
internal/library/           IDs, discovery, hashing, copy, Git add/update
internal/scope/             preset expansion and global/project effective views
internal/link/              reconcile plans, symlinks, cleanup/adopt recovery
internal/status/            read-only drift/unmanaged/orphan summaries
internal/updatecheck/       remote-ref checks and cache
internal/app/               use-case orchestration and transaction boundaries
internal/migrate/           scan/adopt and legacy migration workflows
internal/tui/               Bubble Tea views; no business writes
cmd/                        Cobra parsing/output; no business writes
```

### Task 1: Shared configuration, locking, and agent registry

**Files:**
- Replace: `pkg/config/global.go`
- Replace: `pkg/config/project.go`
- Create: `pkg/config/model.go`
- Create: `pkg/config/paths.go`
- Create: `pkg/config/validate.go`
- Create: `pkg/config/store.go`
- Create: `pkg/config/lock.go`
- Create: `pkg/config/config_test.go`
- Replace: `internal/agent/interface.go`
- Replace: `internal/agent/cursor.go`
- Replace: `internal/agent/claude.go`
- Replace: `internal/agent/copilot.go`
- Replace: `internal/agent/codex.go`
- Replace: `internal/agent/windsurf.go`
- Delete later in Task 9: `internal/agent/managed.go`, `internal/agent/util.go`
- Create: `internal/agent/registry_test.go`

- [ ] **Step 1: Write failing schema and validation tests**

Cover empty config defaults, round-trip YAML, structured `Ref`, full `resolved`, project common plus `agent_bindings`, unique project names/canonical paths, valid binding keys, library/preset references, and pending-operation round trips.

Use these public shapes:

```go
type Ref struct { Kind, Value string }
type Skill struct {
    ID, Name, Description string
    Source, SourcePath string
    Ref *Ref
    Resolved, Hash string
}
type Binding struct { Presets, Skills []string }
type Project struct {
    Name, Path string
    Agents []string
    Binding
    AgentBindings map[string]Binding `yaml:"agent_bindings,omitempty"`
}
type Config struct {
    Library struct { Skills []Skill `yaml:"skills"` } `yaml:"library"`
    Presets []Preset `yaml:"presets,omitempty"`
    Agents map[string]Binding `yaml:"agents,omitempty"`
    Projects []Project `yaml:"projects,omitempty"`
    PendingOperations []PendingOperation `yaml:"pending_operations,omitempty"`
}
```

- [ ] **Step 2: Run RED tests**

Run: `go test ./pkg/config ./internal/agent`
Expected: FAIL because the new schema/store/registry do not exist.

- [ ] **Step 3: Implement paths, schema, and validation**

`Paths` must honor `AIKIT_HOME`; resolve config, lock, library, cache, and update-cache locations. Validation must reject unsafe IDs/names/source paths, duplicate projects/presets/skills, missing references, invalid Agent names, and bindings for Agents not declared by a project.

- [ ] **Step 4: Implement atomic store and lock**

Expose a lock-scoped transaction that permits multiple durable checkpoints while retaining the same process lock:

```go
type Store struct { Paths Paths }
func (s Store) Load(ctx context.Context) (*Config, error)
func (s Store) Save(ctx context.Context, cfg *Config) error
type Tx struct { Config *Config }
func (tx *Tx) Checkpoint() error
func (s Store) WithLock(ctx context.Context, fn func(*Tx) error) error
```

Save through same-directory temp file + fsync + rename. `WithLock` acquires `config.lock`, reloads after lock acquisition, and supports context cancellation. `Checkpoint` validates and atomically persists the current in-memory config without releasing the lock, allowing workflows to save ledger+pending operation before filesystem work and save again after clearing completed operations. An error after the first checkpoint must leave the durable pending operation intact.

- [ ] **Step 5: Implement canonical Agent registry**

Expose `ByName`, `All`, `NormalizeLegacyName`, `GlobalSkillDir(home)`, and `ProjectSkillDir(project)`. Agents do not install/copy content.

- [ ] **Step 6: Run GREEN tests and commit**

Run: `go test ./pkg/config ./internal/agent`
Expected: PASS.

Commit: `feat: add global config and agent registry`

### Task 2: Central library and safe source handling (Wave 1A)

**Files:**
- Create: `internal/library/id.go`
- Create: `internal/library/hash.go`
- Create: `internal/library/discover.go`
- Create: `internal/library/files.go`
- Create: `internal/library/git.go`
- Create: `internal/library/service.go`
- Create: `internal/library/id_test.go`
- Create: `internal/library/hash_test.go`
- Create: `internal/library/service_test.go`

- [ ] **Step 1: Write failing ID/path/hash tests**

Cover equivalent GitHub shorthand/URL IDs, GitLab subgroup preservation, percent-encoded path segments, invalid name/path rejection, containment checks, deterministic hash records, executable bits, and in-root versus escaping symlinks.

- [ ] **Step 2: Run RED tests**

Run: `go test ./internal/library`
Expected: FAIL for missing library API.

- [ ] **Step 3: Implement discovery and hashing**

Expose:

```go
type Candidate struct { Name, Description, Root, RelativePath, Hash string }
func Discover(root string) ([]Candidate, error)
func HashSkill(root string) (string, error)
func NormalizeSource(raw string) (string, error)
func SafeLibraryPath(root, id string) (string, error)
```

Use lexical and resolved containment checks required by spec §2.4.

- [ ] **Step 4: Implement atomic library copy and local add**

Copy into a sibling temp directory, validate `SKILL.md`, fsync, then rename. Reuse same-name/same-hash entries; otherwise allocate `local/<name>` then `local/<name>-<hash12>`.

- [ ] **Step 5: Implement Git add/update with injectable runner**

Store canonical source, per-skill `source_path`, structured ref, and full object ID. A repo with nested skills must update only the selected path. `UpdateRef` must preserve the old library/config result on checkout, validation, name, or copy failure.

- [ ] **Step 6: Run GREEN tests and commit**

Run: `go test ./internal/library`
Expected: PASS.

Commit: `feat: add central skill library`

### Task 3: Scope planning, symlink reconciliation, recovery, and status (Wave 1B)

**Files:**
- Create: `internal/scope/effective.go`
- Create: `internal/scope/effective_test.go`
- Create: `internal/link/types.go`
- Create: `internal/link/plan.go`
- Create: `internal/link/reconcile.go`
- Create: `internal/link/recovery.go`
- Create: `internal/link/adopt.go`
- Create: `internal/link/link_test.go`
- Create: `internal/link/recovery_test.go`
- Create: `internal/status/status.go`
- Create: `internal/status/status_test.go`

- [ ] **Step 1: Write failing effective-view tests**

Cover preset expansion, direct/preset deduplication, project common plus per-Agent binding, same-ID global/project suppression, global disable causing project-link creation, same-name/different-ID scope conflicts, and manually corrupted config.

- [ ] **Step 2: Write failing reconcile/status tests**

Cover missing/correct/wrong links, real directory/file, external symlink, internal/external broken links, missing library, unmanaged, orphaned link, dry-run, scoped sync, and non-zero partial results.

- [ ] **Step 3: Write failing cleanup/adopt recovery matrix tests**

Cover every disk combination in spec §5.1, including temp creation failure, first/second rename failures, successful rollback, failed rollback, fingerprints changed externally, and cleanup refusing later user content.

- [ ] **Step 4: Run RED tests**

Run: `go test ./internal/scope ./internal/link ./internal/status`
Expected: FAIL for missing APIs.

- [ ] **Step 5: Implement pure scope and reconciliation plans**

Planning must be side-effect-free and return typed actions/issues. Execution accepts a scope and never processes unrelated pending operations. Dry-run invokes planning only.

- [ ] **Step 6: Implement durable cleanup/adopt recovery**

Use `Lstat`/`Readlink` lexical ownership, operation fingerprints, same-parent temp/backup paths, and the exact recovery matrix. Never remove mismatched content.

- [ ] **Step 7: Implement read-only status**

Status reports missing, library-missing, conflict, scope-conflict, unmanaged, orphaned-link, pending-cleanup, and adopt-recovery without modifying config or disk.

- [ ] **Step 8: Run GREEN tests and commit**

Run: `go test ./internal/scope ./internal/link ./internal/status`
Expected: PASS.

Commit: `feat: add skill reconciliation and status`

### Task 4: Remote update checking and cache (Wave 1C)

**Files:**
- Create: `internal/updatecheck/types.go`
- Create: `internal/updatecheck/cache.go`
- Create: `internal/updatecheck/checker.go`
- Create: `internal/updatecheck/updatecheck_test.go`

- [ ] **Step 1: Write failing cache/check tests**

Cover key `(canonical source, ref.kind, ref.value)`, separate branches in one repo, full object IDs, 10-minute TTL, forced refresh, pinned refs skipped, failed fetch reported without blocking other entries, and offline mode making zero runner calls.

- [ ] **Step 2: Run RED tests**

Run: `go test ./internal/updatecheck`
Expected: FAIL for missing checker.

- [ ] **Step 3: Implement checker and atomic cache**

Use an injectable Git runner. Checker is read-only with respect to library/config. Cache writes use temp+rename and tolerate corrupt cache by rebuilding it.

- [ ] **Step 4: Run GREEN tests and commit**

Run: `go test ./internal/updatecheck`
Expected: PASS.

Commit: `feat: add skill update checker`

### Task 5: Application service and transaction semantics

**Files:**
- Create: `internal/app/app.go`
- Create: `internal/app/library.go`
- Create: `internal/app/bindings.go`
- Create: `internal/app/presets.go`
- Create: `internal/app/projects.go`
- Create: `internal/app/sync.go`
- Create: `internal/app/update.go`
- Create: `internal/app/app_test.go`
- Create: `internal/app/projects_test.go`

- [ ] **Step 1: Write failing use-case tests**

Cover add-only, add+enable, global/common/per-Agent enable/disable, preset mutation validating all affected scopes, project add/edit/remove, global binding reconciling affected projects, path rebind cleanup, remove reference protection/force, sync scopes/dry-run, lock-scoped checkpoint-before-filesystem and checkpoint-after-cleanup, update confirmation inputs, rollback, and exit-result classification.

- [ ] **Step 2: Run RED tests**

Run: `go test ./internal/app`
Expected: FAIL for missing facade.

- [ ] **Step 3: Implement facade with dependency interfaces**

Expose one `App` used by both frontends. All mutations run in `Store.WithLock`, validate the final state, call `tx.Checkpoint()` to persist pending operations together with ledger changes, then reconcile, clear completed operations, and checkpoint again before releasing the lock. No command-specific output in this package.

Freeze these frontend-facing interfaces before Wave 3 begins so Tasks 6-8 compile independently without editing each other's packages:

```go
type Service interface {
    Snapshot(context.Context, StatusRequest) (Snapshot, error)
    Add(context.Context, AddRequest) (Result, error)
    Enable(context.Context, BindingRequest) (Result, error)
    Disable(context.Context, BindingRequest) (Result, error)
    Sync(context.Context, SyncRequest) (Result, error)
    Update(context.Context, UpdateRequest) (Result, error)
    EditProject(context.Context, ProjectEditRequest) (Result, error)
    RemoveProject(context.Context, ProjectRemoveRequest) (Result, error)
}
type MigrationService interface {
    Scan(context.Context, ScanRequest) (ScanResult, error)
    Migrate(context.Context, MigrateRequest) (MigrateResult, error)
}
```

- [ ] **Step 4: Implement project and preset semantics**

Project edit supports canonical-path uniqueness, common/per-Agent scopes, remove-Agent cleanup, rename, and path rebind with explicit approval passed by caller.

- [ ] **Step 5: Implement update and removal semantics**

Remote updates are selected explicitly; local entries skip. `--ref` uses typed prefixes. Force removal prunes all references and reconciles all affected targets.

- [ ] **Step 6: Compose read-only status with update checking**

`App.Snapshot` always collects filesystem status first. Unless `Offline`, it asks `updatecheck` using the 10-minute cache/default-fetch contract, adds updates/check-failed to the snapshot, and never updates library/config. A fetch failure preserves all other status results and produces the spec-defined exit classification.

- [ ] **Step 7: Run package and Wave 1 integration tests; commit**

Run: `go test ./pkg/config ./internal/agent ./internal/library ./internal/scope ./internal/link ./internal/status ./internal/updatecheck ./internal/app`
Expected: PASS.

Commit: `feat: add skills manager application service`

### Task 6: Scan, adopt, and legacy migration (Wave 3A)

**Files:**
- Create: `internal/migrate/scan.go`
- Create: `internal/migrate/adopt.go`
- Create: `internal/migrate/legacy.go`
- Create: `internal/migrate/scan_test.go`
- Create: `internal/migrate/legacy_test.go`
- Modify: `internal/app/app.go` only to inject/export the migration service interface agreed in Task 5

- [ ] **Step 1: Write failing scan/adopt tests**

Cover all five Agents, selected registered project, exact Agent-binding placement, same-name/hash reuse, stable local IDs, existing aikit links, non-TTY filters, unmanaged selection, and adopt recovery.

- [ ] **Step 2: Write failing migration tests**

Cover catalog-only, explicit repeatable project paths, cwd project, empty/non-empty config merges, idempotency, ambiguous legacy source paths, conflicts, no deletion, default pending adopt, explicit adopt, and installed content differing from cache.

- [ ] **Step 3: Run RED tests**

Run: `go test ./internal/migrate`
Expected: FAIL.

- [ ] **Step 4: Implement scan/adopt and migration**

Migration must never scan the whole disk. Commit each successful entry atomically with its references, preserve partial successes, and produce typed summary counts.

- [ ] **Step 5: Run GREEN tests and commit**

Run: `go test ./internal/migrate ./internal/app`
Expected: PASS.

Commit: `feat: add scan adopt and migration workflows`

### Task 7: Cobra CLI (Wave 3B)

**Files:**
- Replace: `cmd/root.go`
- Replace: `cmd/add.go`
- Replace: `cmd/list.go`
- Replace: `cmd/remove.go`
- Replace: `cmd/sync.go`
- Create: `cmd/enable.go`
- Create: `cmd/disable.go`
- Create: `cmd/preset.go`
- Create: `cmd/project.go`
- Create: `cmd/status.go`
- Create: `cmd/update.go`
- Create: `cmd/scan.go`
- Create: `cmd/migrate.go`
- Keep/modify: `cmd/version.go`
- Create: `cmd/root_test.go`
- Create: `cmd/commands_test.go`

- [ ] **Step 1: Write failing command contract tests**

Construct commands with injected fake `internal/app` and TUI launcher interfaces. Cover command tree, flags, non-TTY missing-argument errors, cwd project inference, scope flag meaning, read-only status/offline/check-failed behavior, update exit code 2, non-TTY update/path-rebind `--yes`, dry-run, and no export/import/catalog/web commands. For every interactive command with missing required input, add a TTY test proving it launches the corresponding full-screen TUI view and does not call a mutation before user confirmation.

- [ ] **Step 2: Run RED tests**

Run: `go test ./cmd`
Expected: FAIL for missing new command tree.

- [ ] **Step 3: Implement Cobra parsing/output**

Commands translate flags into app requests and render typed results. No direct YAML/filesystem writes. Make `Execute()` return an exit code to `main` without calling `os.Exit` inside tests. Inject `IsTTY` and `LaunchTUI(initialView, initialAction)`; parameter-complete commands execute directly, TTY incomplete commands route to the matching TUI view, and non-TTY incomplete commands fail immediately.

- [ ] **Step 4: Add CLI smoke test**

With temp `AIKIT_HOME` and fake home/project directories: local add → global enable → project add → per-Agent enable → status → disable → sync.

- [ ] **Step 5: Run GREEN tests and commit**

Run: `go test ./cmd`
Expected: PASS.

Commit: `feat: replace cli with global skills commands`

### Task 8: k9s-style Bubble Tea TUI (Wave 3C)

**Files:**
- Replace: `internal/tui/select.go`
- Create: `internal/tui/model.go`
- Create: `internal/tui/views.go`
- Create: `internal/tui/keys.go`
- Create: `internal/tui/commands.go`
- Create: `internal/tui/model_test.go`
- Create: `internal/tui/views_test.go`

- [ ] **Step 1: Write failing model/update tests**

Cover view keys 1-5, navigation/filter/help/escape, library detail, Agent global view, project common/Agent scopes, preset application target selection, status/unmanaged adopt preview, update multi-select/confirmation, async update results, and no writes after cancellation.

- [ ] **Step 2: Run RED tests**

Run: `go test ./internal/tui`
Expected: FAIL for missing model.

- [ ] **Step 3: Implement the model and renderers**

Use Bubble Tea commands to call the injected app interface; never block `Update`. TUI startup triggers cached/background update check without blocking first render. Fit narrow terminals and expose current error/status in a stable footer.

- [ ] **Step 4: Implement mutation confirmations and project editing**

Support install source/path overlay, remove/force, update/ref rollback, project name/path/Agent editing, scan/adopt table, and update selection. Esc/Ctrl-C before confirmation performs no mutation.

- [ ] **Step 5: Run GREEN tests and commit**

Run: `go test ./internal/tui`
Expected: PASS.

Commit: `feat: add global skills manager tui`

### Task 9: Remove legacy product and wire the binary

**Files:**
- Modify: `main.go`
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`
- Replace: `scripts/test-e2e.sh`
- Delete: `cmd/catalog.go`
- Delete: `cmd/catalog_add.go`
- Delete: `cmd/catalog_list.go`
- Delete: `cmd/catalog_remove.go`
- Delete: `cmd/catalog_sync.go`
- Delete: `cmd/catalog_ui.go`
- Delete: `cmd/catalog_update.go`
- Delete: `cmd/init.go`
- Delete: `cmd/publish.go`
- Delete: `internal/catalog/catalog.go`
- Delete: `internal/web/handler.go`
- Delete: `internal/web/server.go`
- Preserve entire working-tree directory: `internal/web/frontend/` during this task because `tsconfig.app.tsbuildinfo` has a pre-existing user modification; removal of tracked frontend files must use explicit per-file patches that exclude this path and must not stage the user file
- Delete obsolete: `internal/asset/`, `internal/discovery/`, `internal/skill/`, `internal/source/`

- [ ] **Step 1: Add a failing binary smoke check**

Assert `aikit --help` lists the new surface and omits catalog/publish/web/export/import.

- [ ] **Step 2: Remove legacy registration/code and wire dependencies**

Remove Gin/frontend build from Makefile and Go modules. Wire real config/library/link/status/update/migrate/app dependencies into root command and TUI.

- [ ] **Step 3: Update CI and e2e script**

CI runs `go test ./...`, `go vet ./...`, and `go build ./...` without Node. E2E uses temp `AIKIT_HOME` and temp HOME; it never touches actual IDE directories.

- [ ] **Step 4: Run verification and commit**

Run formatting with `git ls-files -z '*.go' | xargs -0 gofmt -w`, then run `go mod tidy`, `go test ./...`, `go vet ./...`, and `go build ./...` as separate commands.
Expected: all commands exit 0.

Before committing, run `git status --short internal/web/frontend/tsconfig.app.tsbuildinfo` and verify it still shows only the pre-existing unstaged modification. Stage explicit implementation paths; never use `git add -A` or stage `internal/web/frontend`.

Commit: `refactor: remove legacy catalog product`

### Task 10: Full acceptance, race testing, and manual TUI walkthrough

**Files:**
- Modify tests only when an acceptance gap is proven
- Update: `README.md`
- Update: `docs/design-zh.md`

- [ ] **Step 1: Run the full automated suite fresh**

Run:

```bash
go test -race ./...
go vet ./...
go build ./...
bash scripts/test-e2e.sh
```

Expected: all exit 0, no race reports.

Also run `git status --short internal/web/frontend/tsconfig.app.tsbuildinfo` and confirm the user modification remains unstaged and unchanged from the turn-start diff.

- [ ] **Step 2: Execute destructive-safety acceptance cases in temp roots**

Exercise external directories/symlinks, broken links, missing library, interrupted adopt states, pending cleanup, path rebind, concurrent config mutations, migration conflict, and rollback. Confirm no path outside temp HOME/AIKIT_HOME/project changes.

- [ ] **Step 3: Manually walk the TUI in a temp environment**

Verify list movement, view switching, filters, project common/Agent scopes, project editing, unmanaged adopt preview/cancel/confirm, update confirmation/cancel, help, narrow terminal rendering, and quit.

- [ ] **Step 4: Update user documentation**

Document single-machine scope, central library, add/scan/adopt distinction, global/project/per-Agent bindings, status categories, updates/rollback, migration, and explicit non-goals.

- [ ] **Step 5: Request final code review**

Review the complete range from `aef994b` (approved spec) to HEAD for spec compliance, adjacent regressions, filesystem safety, and test gaps. Fix every Critical/Important finding and rerun the full verification suite.

- [ ] **Step 6: Commit final acceptance changes**

Commit: `docs: document global skills manager`
