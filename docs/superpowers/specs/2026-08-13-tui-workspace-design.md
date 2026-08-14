# aikit Workspace TUI Redesign

Date: 2026-08-13
Status: Approved for implementation

## 1. Goal

Replace the current shortcut-heavy table TUI with an English-first, workspace-oriented interface that provides the management capabilities users expect from `skills-manager` while remaining native to a terminal.

The redesign must prioritize clarity and operability over information density. It must support both mouse and keyboard input, expose scattered existing skills as explicit migration candidates, and keep all business writes behind `internal/app` and `internal/migrate`.

## 2. Product principles

- Feature parity is a capability target, not a visual-copy target.
- Common actions must take no more than three deliberate interactions.
- The default screen shows only the information needed for the current decision.
- Secondary metadata and actions use progressive disclosure.
- A row is the mouse hit target; users are not required to click small glyphs.
- Every mouse action has a keyboard equivalent.
- Color is supplementary; every state also has a symbol or short label.
- Startup discovery is read-only, local-only, cancellable, and incremental.
- No preview, selection, cancellation, or startup scan may mutate config or files.
- A mutation is submitted once; repeated key presses or clicks while busy are ignored.
- All visible product copy, help text, tests, and new documentation are English.

## 3. Information architecture

The top-level navigation is:

1. Overview
2. Library
3. Workspaces
4. Presets
5. Migration
6. Status

The normal layout uses two panes:

```text
+ aikit -------------------------------- / Search   Refresh +
| Overview  Library  Workspaces  Presets  Migration  Status |
+-----------------------+------------------------------------+
| Skills                | code-review                        |
|                       |                                    |
|  managed brainstorming| Git - main - Managed               |
|  unmanaged code-review| Claude - Codex                     |
|  update go-expert     |                                    |
|  conflict rust-helper | [Enable] [Adopt] [...]             |
+-----------------------+------------------------------------+
| 4 unmanaged - 1 conflict                   Select - Open   |
+------------------------------------------------------------+
```

The left pane is the current collection. The right pane is contextual detail. Workspaces may temporarily show a tree in the left pane. Narrow terminals switch to a single-pane stack with breadcrumbs instead of compressing columns.

The footer contains only two to four context-relevant actions. Full help is opened on demand. Batch actions appear only while rows are selected. No page permanently displays a shortcut wall.

## 4. Startup inventory

Every TUI launch performs the following sequence:

1. Load the config and central-library snapshot.
2. Render the UI immediately.
3. Start a local read-only scan of every supported agent's global skill directory.
4. Scan every registered project's declared agent skill directories.
5. Merge results incrementally without moving the cursor or changing scroll position.

Startup inventory must not:

- contact Git or any other network service;
- acquire a mutation lock;
- checkpoint config;
- import, adopt, reconcile, delete, or recover anything;
- follow an untrusted path outside an authorized root.

One failed root produces a structured issue and does not stop the other roots. Remote update checks remain a separate explicit action.

## 5. Inventory identity and states

An inventory item is uniquely identified by its origin, scope, agent, project, and absolute target path. Skill name or skill ID alone is not a selection key.

Each item exposes:

- origin and target path;
- global or project scope;
- agent and optional project;
- discovered skill metadata and content hash;
- matching central-library skill, if any;
- management and diagnostic state;
- recommended non-binding action.

States are:

- `Managed`
- `Unmanaged`
- `Same content`
- `Name conflict`
- `Broken link`
- `Drifted`
- `Update available`
- `Pending recovery`
- `Error`

Symbols may accompany these labels, but labels remain available in detail and accessibility-oriented output.

## 6. Migration and adoption

Migration is a dedicated top-level page. It distinguishes these actions:

- `Import`: copy into the central library while leaving the source directory unchanged and creating no binding.
- `Adopt`: import, create the exact inferred binding, and replace only the confirmed source path with a managed link.
- `Link existing`: reuse a byte-identical library entry, create the exact binding, and replace only the confirmed source path.
- `Compare`: inspect metadata, file list, hash, and content differences.
- `Ignore`: skip the item for the current scan or persist an ignored origin in a later explicit feature.

The flow is selection, action, exact change preview, confirmation, execution, and per-item result. Confirmation shows library additions, bindings, and every filesystem path that will be replaced. Same-name different-content candidates are never overwritten automatically.

The old `catalog.yaml` and `.aikit.yaml` workflow appears under `Migration > Legacy Config` with Preview, Migrate, and Migrate and Adopt actions. Migration never deletes legacy config or cache files.

## 7. Daily management

### 7.1 Library

Library supports source add and preview, local and Git skills, search, state/source filters, content and file-tree preview, enable/disable, update, ref change, and safe remove. Multi-selection enables batch enable, disable, update, and remove. Referenced skills are not force-removed without an explicit second decision.

### 7.2 Workspaces

Workspaces contains Global, Agents, and Projects. A selected workspace shows enabled, available, suppressed, conflicted, inherited, and preset-provided skills. It supports exact enable/disable, preset application, effective-scope inspection, and sync preview.

Global changes preview every affected agent and registered project. Project edits preview cleanup of the old path before reconcile of the new path. Unknown project content is never deleted.

### 7.3 Presets

Presets supports create, edit members, duplicate, rename, delete, inspect usage, and apply to Global, Agent, Project Common, or Project Agent scope. Member editing shows the library as a selectable list.

### 7.4 Updates

Remote checks run only on explicit request. Results show current and remote full identities, ref, cache/offline state, and per-skill errors. Updates support selection, exact preview, confirmation tokens, and batch application while preserving old library and ledger state on failure.

### 7.5 Status

Status shows broken links, external/unmanaged entries, drift, scope conflicts, missing projects, pending operations, and update-check failures. Each issue exposes only relevant actions such as Retry, Compare, Review Recovery, Sync, or Copy Error.

## 8. Interaction model

Keyboard basics are arrow keys or `j/k`, `Tab`, `Enter`, `Space`, `/`, `Esc`, `Ctrl+K`, and `?`. Page-specific shortcuts may exist but are never the only discoverable route.

Mouse support uses Bubble Tea cell-motion events:

- clicking tabs switches pages;
- clicking a row selects it;
- clicking a checkbox toggles batch selection;
- clicking contextual buttons opens the same preview used by keyboard actions;
- the wheel scrolls the focused collection or detail;
- modal buttons are clickable.

The product must not depend on hover, right-click, or double-click behavior. Focus is visually explicit. Busy mutation state rejects all repeated action input except cancellation when cancellation is safe.

## 9. Density and progressive disclosure

- Skill rows show name plus one primary state by default.
- The detail pane initially shows source type/ref, management state, enabled locations, and at most three visible actions.
- Secondary actions use a More menu.
- `SKILL.md`, file trees, hashes, paths, warnings, and exact plans open on demand.
- A dashboard summary such as `4 skills need attention` leads to categorized results.
- Success uses a short non-blocking notice; errors and pending recovery remain visible.
- Background scan updates never reorder the active row out from under the user. Stable sorting is applied between explicit refreshes or while no row is focused.

## 10. Configuration surface

`Ctrl+K > Show Configuration` displays resolved paths:

```text
Config:  $AIKIT_HOME/config.yaml
Library: $AIKIT_HOME/library/skills
Cache:   $AIKIT_HOME/cache
```

Without `AIKIT_HOME`, the displayed config is `~/.aikit/config.yaml`. Actions are Copy Path, Validate, Reload, and Edit in `$EDITOR`. The TUI does not implement a partial YAML editor.

## 11. Architecture boundaries

`internal/tui` owns presentation state, focus, viewport, hit regions, and translation of events into application requests. It never writes config, library content, links, or Git state directly.

`internal/migrate` owns read-only inventory and exact-origin migration/adoption requests. Read-only inventory needs a structured API that scans all registered projects, reports per-root progress/issues, and preserves origin identity.

`internal/app` owns all use-case previews and mutations. CLI and TUI call the same operations. Existing mutation recovery and ledger-driven library transactions remain authoritative.

The TUI should be split by responsibility rather than growing a single model file: application model/messages, navigation and focus, inventory view model, mouse hit testing, rendering/layout, commands, overlays, and styles.

## 12. Recovery and errors

Startup inventory reports pending operations but does not recover them automatically. New mutations are blocked until the user opens Review Recovery and explicitly resumes or rolls back, using the established recovery APIs.

Errors are structured by root or item and retain operation/path context. A short summary is shown inline; full diagnostics open on demand. An item failure never discards successful results from other items.

## 13. Acceptance criteria

- Every launch scans all global agent roots and all registered projects without network or writes.
- Main content appears before inventory completes.
- Mouse and keyboard can complete every primary workflow.
- Main pages remain useful without memorized shortcuts.
- Overview, Library, Workspaces, Presets, Migration, and Status are all navigable and functional.
- Selection is keyed by exact origin and path.
- Import, Adopt, and Link existing are distinct and previewed.
- Cancelling any preview causes zero mutations.
- Same-name different-content candidates are never silently overwritten.
- At most one mutation runs at a time; repeated clicks do not duplicate it.
- Details, help, and exact plans are progressively disclosed.
- Narrow layouts remain operable.
- All visible copy is English.
- Config, library, cache, and resolved `AIKIT_HOME` paths are visible in the Configuration surface.
- TUI tests cover keyboard and mouse parity, hit regions, responsive layout, automatic inventory, cancellation, exact-origin adoption, busy gating, focus preservation, and partial errors.

## 14. Legacy cleanup

The runtime product is CLI plus TUI. `internal/web`, obsolete Catalog/Web UI assets and documentation, empty legacy package directories, and generated frontend artifacts are removed after the replacement TUI passes its integration tests. The current global-manager specification, implementation plan, README, and local E2E coverage are updated and retained.
