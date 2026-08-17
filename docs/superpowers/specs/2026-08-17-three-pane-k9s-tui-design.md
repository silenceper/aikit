# Three-pane K9s-style TUI Design

Date: 2026-08-17
Status: Approved interaction and visual direction; pending implementation plan
Baseline: `090fe48`

## Goal

Make aikit's TUI feel like one coherent application rather than a collection of
workflow-specific modes. The chosen direction combines a Lazygit-style stable
three-pane workspace with K9s-style framed regions, command navigation, and
contextual shortcuts.

The redesign must make these common tasks obvious without prior documentation:

1. add a skill from a local directory, GitHub, or skills.sh;
2. inspect, select, update, enable, or remove library skills;
3. register a project and configure its agents, skills, and presets;
4. create a preset, edit its members, and apply it to an exact target; and
5. identify and resolve status, migration, or recovery work.

This design supersedes the sparse-divider and two-line-collection presentation
rules in `2026-08-14-modern-tui-visual-design.md`. It preserves the previously
approved typed preview, recovery, path-safety, and mutation boundaries.

## Chosen structure

At wide terminal widths, the application uses three stable panes:

1. **Navigation** — permanent top-level destinations;
2. **Collection** — the current list of skills, projects, presets, or issues;
3. **Detail** — live context, usage, diagnostics, and current actions.

The permanent navigation contains exactly:

- Overview;
- Library;
- Workspaces;
- Presets; and
- Status.

Workspaces contains both global agents and registered projects. Migration and
Configuration remain available under a visually separate `Tools` section and
through the command palette; they do not compete with daily destinations.

The app bar shows `aikit`, the active context, and compact busy/scan state. The
footer shows only shortcuts valid for the focused pane. A `Ctrl+P` command
palette and the K9s-compatible `:` alias provide direct jumps to destinations
and actions. `/` filters the focused collection. `?` opens contextual help.

## Visual system

Use K9s-style framed panels with disciplined nesting:

- draw one frame around each of Navigation, Collection, and Detail;
- integrate each pane title into its top border;
- draw one frame around overlays;
- draw one frame around the app bar and one around the contextual footer;
- do not box individual rows, metrics, badges, buttons, or detail groups; and
- do not nest cards inside the Detail frame.

The focused pane receives the strongest border/accent. The selected row uses a
cursor marker and restrained background or weight. Success, warning, and error
colors remain semantic, but every state also has a textual marker for
`NO_COLOR` and reduced-color terminals.

Control notation is consistent:

- `[ ]` and `[x]` are selection checkboxes;
- `[Enable]`, `[Update]`, and `[More]` are actions;
- `>` marks the focused row or control; and
- focus is never represented by changing square brackets to braces.

Unicode box-drawing characters are preferred. The ASCII fallback uses `+`,
`-`, and `|` with the same geometry. Rendered geometry and mouse hit regions
must come from the same layout primitives.

## Responsive behavior

- **96 columns and above:** render Navigation, Collection, and Detail together.
- **60-95 columns:** render compact Navigation plus Collection; Detail opens as
  a full workspace page or bounded overlay and retains a breadcrumb back path.
- **Below 60 columns:** render one pane at a time with breadcrumb navigation.
  App bar, active pane frame, action row, and footer remain visible.

At all supported widths, content is clipped or wrapped by display cells and
graphemes rather than runes. A long skill description, path, diagnostic, CJK
text, or emoji must not escape its owning frame. Detail scrolling is independent
from collection scrolling.

## Selection and action model

Selection never executes a mutation.

In the main Library collection:

- clicking anywhere on a skill row, clicking its checkbox, or pressing Space
  toggles the same multi-selection state;
- the active row also updates the live Detail pane;
- Enter moves focus to Detail; and
- visible batch actions operate on the exact selected identities.

In nested single-choice pickers, clicking a row selects the value but does not
submit or launch a mutation. A separate `[Apply]` or `[Confirm]` action advances
the workflow. Picker selection must not reuse the Library batch-toggle path.

Keyboard and mouse call the same semantic action dispatcher. No visible mouse
action may be unreachable from the keyboard, and no visible keyboard action may
lack a mouse hit target. Busy mutations reject duplicate submissions. `Esc`
only cancels the current modal or goes back one level; `Ctrl+Q` is the sole
quit chord from a normal page.

## Workspace behavior

### Overview

Overview is a compact operational summary, not a separate dashboard design.
It shows Library, Projects, Presets, and Attention counts plus a framed
`Needs attention` collection. Selecting an item opens the owning destination.
Startup remains offline and read-only; update state is `Not checked` until an
explicit refresh.

### Library

The Collection pane supports source/state filters and stable multi-selection.
The Detail pane shows source, ref/resolved identity, state, summary, usage, and
diagnostics. Its primary actions are limited to three; remaining operations are
under More.

Primary workflows are:

- Add source — one input accepting local paths, GitHub URLs/shorthand, and
  skills.sh URLs; discovery produces a candidate checklist before import;
- Enable/Disable — choose an exact global, project-common, or project-agent
  target and review the plan;
- Update/Change ref — preserve exact confirmation tokens and rollback behavior;
- Remove — show references and require the established second force decision;
  and
- Compare — show structured metadata, file, and symlink differences.

### Workspaces

The Collection pane shows registered projects followed by global agents. A
project Detail pane shows its path, state, agents, direct skills, inherited or
preset-provided skills, and next sync state.

The three project primary actions are `[Add skill]`, `[Apply preset]`, and
`[More]`. More contains rename, manage agents, change directory, sync preview,
and remove project. Project creation remains path-first: one directory input,
derived name, detected agents, typed preview, and explicit confirmation.

Opening a project agent or Common scope reuses the Library skill collection,
but the header and action plan retain the exact project/scope identity.

### Presets

The Collection pane lists presets and usage counts. Detail shows members and
every current target. Primary actions are `[Edit members]`, `[Apply]`, and
`[More]`. Create, duplicate, rename, and delete remain available under More.

Editing members opens the Library checklist with current membership preselected.
Applying a preset always chooses and displays one exact target: global agent,
project common, or project agent. No preset operation depends on encoded text
or hidden inferred scope.

### Status, Migration, and Recovery

Status groups issues by severity and owning scope. Detail preserves complete
underlying errors and paths. Refresh is explicit and may use the network only
when the user requests it.

Migration remains a Tool workspace using exact origin/target selectors. Startup
inventory is offline and does not mutate. Pending recovery gates every ordinary
mutation and routes to one explicit Review Recovery surface. The TUI exposes
only recovery actions supported by the typed application result.

## Safety and error behavior

- Every write continues through typed app or migration services.
- Add, binding, preset, project, update, remove, migration, sync, and recovery
  operations retain their preview/confirm boundaries.
- Confirmations show exact scopes, targets, paths, conflicts, warnings, and
  structured issues; long bodies scroll without hiding title or actions.
- Cancel produces zero mutation and restores the prior pane, row identity,
  selection, and scroll anchor.
- Partial results remain visible with authoritative aggregate `Changed`
  semantics; preflight items are never described as committed changes.
- Unknown project or skill content is never deleted.
- Incremental inventory updates preserve selection by stable key rather than
  row index.

## Implementation boundaries

The redesign should remain inside `internal/tui` unless a focused typed preview
gap is proven. Existing `app.Service` and `MigrationService` contracts are the
business authority. Frontend code must not duplicate scope, preset, source,
project, recovery, or filesystem planning.

Recommended presentation boundaries:

- `layout.go`: pure responsive three-pane and overlay rectangles;
- `panel.go`: frame glyphs, titles, clipping, and shared hit geometry;
- `rows.go`: semantic collection projection and stable identities;
- `render.go`: shell composition and domain-specific detail sections;
- `actions.go`: semantic selection/action dispatch;
- `input_keyboard.go` and `input_mouse.go`: translate input into shared actions;
  and
- `model.go`: navigation, focus, asynchronous generation, and result state.

The first implementation task must remove the known picker regression in which
a Library-owned nested picker click can dispatch the main Library toggle path.
Main Library row toggling remains intentional; picker choice and submission are
separate actions.

## Testing and acceptance

Use strict RED/GREEN cycles and verify:

1. all five permanent destinations and both Tools routes;
2. wide, compact, and narrow pane geometry at 120, 96, 95, 80, 60, 59, 38,
   and 24 columns, plus heights 8, 12, 16, and 30;
3. Unicode and ASCII framed geometry with no content or hit-region escape;
4. main Library whole-row/checkbox/Space equivalence;
5. nested picker selection without implicit preview submission or mutation;
6. keyboard/mouse parity for every visible row, action, palette item, modal,
   paging control, and back path;
7. Library add, project configure, and preset member/apply workflows in no more
   than three deliberate interactions after choosing the owning item;
8. exact preview/confirm request parity and cancel zero-write;
9. stable row/scroll identity during filter, snapshot, and inventory updates;
10. long real-world descriptions, paths, diagnostics, CJK, and emoji bounded
    inside frames; and
11. dark, light, `NO_COLOR`, reduced-color, and ASCII output.

Final verification includes `go test ./...`, `go test -race ./...`, `go vet
./...`, native and Windows builds, E2E, formatting, and `git diff --check`.

## Non-goals

- Do not copy K9s resource density, terminology, or Kubernetes mental model.
- Do not add a Web UI or restore `internal/web`.
- Do not add startup network activity or automatic mutation.
- Do not redesign backend storage, link journals, or config schema.
- Do not expose a shell, editor, clipboard, or arbitrary command execution.
- Do not add decorative nested cards, gradients, or animation that reduce
  terminal compatibility.
