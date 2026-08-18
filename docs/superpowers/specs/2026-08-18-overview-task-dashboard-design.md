# Overview Task Dashboard Design

Date: 2026-08-18
Status: Approved
Baseline: `3bdef14`

## Goal

Turn Overview into the primary task dashboard. A user should be able to add a
skill, review concrete updates, import or adopt discovered local skills, and
resolve health issues without first choosing between Status and Migration.

Status and Migration are removed from primary navigation, but their read-only
detail views, typed plans, confirmations, recovery behavior, and CLI commands
remain available. This design changes TUI information architecture only; it
does not weaken mutation, containment, identity, or recovery guarantees.

## Navigation

Primary navigation becomes:

```text
MAIN
  Overview
  Library
  Presets

WORKSPACES
  Global
  Agents
  Projects

TOOLS
  Configuration
```

Status and Migration are not visible in the navigation rail and do not receive
number shortcuts. `1`, `2`, and `3` continue to open Overview, Library, and
Presets respectively.

The hidden views remain routable through:

- a relevant Overview task row;
- `Review health details` in `Ctrl+P`; and
- `Review local skill imports` in `Ctrl+P`.

CLI commands including `status`, `scan`, and `migrate` remain unchanged.

## Overview structure

Overview has four stable task sections.

### Quick actions

Quick actions are always visible, including when all collections are empty:

- `Add skill` opens the existing typed add-source workflow;
- `Add project` opens the existing path-first project workflow; and
- `Create preset` opens the existing preset creation workflow.

The labels are task-oriented. They reuse the same semantic actions and app
requests as Library, Projects, and Presets; Overview does not implement a
second mutation path.

### Updates

Startup remains offline and does not contact Git or update services. Cached
update results may be shown. If no usable result exists, the section displays
`Not checked` and `Check updates`.

After an explicit check, the section lists each updateable skill with its
stable skill ID, current revision, and remote revision. Users may select one or
more rows and choose `Update selected`. The existing batch-update contract
provides complete confirmation tokens and a typed preview before the single
mutation call. Failed checks remain visible as diagnostics and cannot be
selected for update.

### Local skills

The section consumes the automatic offline inventory stream and lists concrete
local candidates. Each row exposes its typed state and recommended action,
including `Import`, `Link existing`, `Adopt`, `Conflict`, or `Error`.

Executable rows may be selected together. Confirmation displays every exact
origin, target path, and planned action. Conflict, drift, error, and pending
recovery rows remain inspectable but cannot be included in an executable
batch. Selection authorization continues to use the backend-provided stable
selector and is revalidated under the mutation lock.

`Review all` opens the hidden Migration detail view at the corresponding stable
row. The Overview never duplicates migration planning.

### Needs attention

This section combines:

- missing, broken, orphaned, or conflicting links;
- missing library or project paths;
- pending recovery operations;
- inventory and update-check failures; and
- other typed Status issues.

The active row offers only actions valid for its type: `Open`, `Sync preview`,
`Adopt`, or `Review recovery`. Unknown files, external links, and conflicting
content are never overwritten automatically. Complex details open the hidden
Status view at the exact composite-key row.

## Layout and progressive disclosure

At 96 columns and wider:

- Quick actions span the top of the Collection area;
- Updates and Local skills are separate side-by-side task sections; and
- Needs attention spans the lower area.

At 60-95 columns the sections are stacked in the same order. Below 60 columns,
only the active section is expanded; the section list and breadcrumb provide
navigation without adding another permanent navigation level.

Each section shows a bounded number of rows and a visible count. `Review all`
opens the complete retained detail view. Row and action hit regions share the
same layout primitives as rendering. Long names, paths, CJK text, and emoji are
clipped by display cells and never cross a panel boundary.

## Focus, selection, and actions

Overview has stable section identities and per-section row keys. Keyboard and
mouse use the same actions:

- `Tab` moves among Navigation, sections, and the active section's actions;
- arrows or `j/k` move within the active section;
- Space or the checkbox toggles only executable update/import rows;
- Enter opens the active task or its detail;
- Escape returns one level and never exits the root TUI; and
- `Ctrl+Q` remains the intentional normal exit chord.

Selections are independent between Updates and Local skills. Incremental
inventory events preserve active section, selected stable key, cursor, and
viewport offset. Rows that become invalid are visibly disabled and removed
from the executable selection set.

## Data flow

Startup remains:

1. load an offline Snapshot;
2. render cached update/status information;
3. stream local inventory per authorized root; and
4. merge events into Overview by stable key and generation.

Explicit `Check updates` is the only Overview action that starts remote update
checks. It stores typed results in the existing Snapshot/update state. Update,
import, adopt, sync, and recovery actions continue through existing app or
migration preview APIs, then confirmation, then one deferred mutation command.

Overview is a projection and dispatcher. It performs no config writes, direct
filesystem planning, Git execution, or recovery itself.

## Error and recovery behavior

- Top-level and per-item errors remain visible together.
- Partial results do not claim completion.
- Canceling preview, selection, input, or confirmation performs zero writes.
- Busy state blocks duplicate keyboard and mouse submissions.
- Pending recovery is promoted to the top of Needs attention and gates ordinary
  mutations until explicit Review Recovery completes.
- Hidden Status and Migration pages remain reachable even if Overview cannot
  render a specialized action for a new issue type.

## Verification

Implementation must prove:

1. Status and Migration are absent from the rail and number shortcuts;
2. both retained views remain reachable from exact Overview rows and `Ctrl+P`;
3. Quick actions use the existing Add, Project, and Preset requests;
4. startup performs no update-network or Git calls;
5. explicit update check shows concrete results and failures;
6. update multi-selection rejects stale, failed, or incomplete tokens;
7. local candidates retain exact selectors and exclude unsafe rows;
8. Overview health actions preserve typed paths, issues, and recovery gates;
9. incremental inventory preserves section, cursor, viewport, and selection;
10. keyboard/mouse parity and render/hit geometry hold at 24, 38, 80, and 120
    columns;
11. cancellation and previews perform zero mutations; and
12. package, full, race, vet, native, Windows, E2E, and real-config read-only
    verification pass.
