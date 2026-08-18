# Pane-owned Actions and Workspace Navigation Design

Date: 2026-08-18
Status: Approved interaction direction; pending implementation plan
Baseline: `ebdbea2`

## Goal

Make every visible action appear in the pane that owns its subject, and remove
the intermediate Workspaces landing page. Users should be able to answer two
questions without trial and error:

1. does this action affect the current collection or the selected item; and
2. where do I go to manage global bindings, agents, or projects?

This design is an incremental amendment to
`2026-08-17-three-pane-k9s-tui-design.md`. It does not change application
services, mutation semantics, previews, confirmations, recovery gates, or the
responsive frame style.

## Navigation structure

The wide navigation rail is grouped as follows:

```text
MAIN
  Overview
  Library
  Presets
  Status

WORKSPACES
  Global
  Agents
  Projects

TOOLS
  Migration
  Configuration
```

`WORKSPACES` is a non-interactive section heading, like `TOOLS`. The former
Workspaces landing page and its `Global / Agents / Projects` collection rows
are removed. Each child is a direct route:

- **Global** opens the all-global-target projection;
- **Agents** opens the supported-agent collection; and
- **Projects** opens the registered-project collection.

The existing `ViewWorkspaces` implementation may remain as the internal owner
of workspace projections. Navigation entries carry the exact initial scope, so
the frontend does not duplicate domain planning. Existing `ViewAgents` and
`ViewProjects` launch aliases continue to resolve to the corresponding direct
routes.

At 60-95 columns, the compact navigation keeps the same grouped destinations
using abbreviated labels where necessary. Below 60 columns, the active route
is shown in the breadcrumb and all three destinations remain directly
available through `Ctrl+P` and keyboard navigation. Back returns through the
workspace scope hierarchy but never recreates the removed landing page. At the
root of Global, Agents, or Projects, `Esc` is a no-op, matching the application's
existing root-back contract.

## Pane-owned actions

The TUI exposes two independent action sets whenever both Collection and Detail
panes are visible.

### Collection actions

Collection actions affect the current list, its selection set, or the page as
a whole. They render inside the Collection frame action row and include:

- add or create operations such as `Add source`, `Create project`, and
  `Create preset`;
- filtering, refresh, and batch operations;
- selection-wide enable, disable, update, or remove operations.

Collection actions remain available when the collection is empty. Empty-state
text and the action row call the same semantic dispatcher.

### Detail actions

Detail actions affect only the active row or the exact scope represented by the
Detail pane. They render inside the Detail frame action row and include:

- opening full content or a nested scope;
- editing, renaming, changing a ref, comparing, or removing the selected item;
- applying a preset or changing bindings for the selected exact target; and
- a contextual `More` menu containing only operations for that item or scope.

On wide layouts, opening the active row is a Detail action. On compact and
narrow collection pages, Enter may open that row's full detail page without
adding an item action to the Collection action row.

An action must not appear in Detail merely because Detail is present. In
particular, `Add source`, `Create project`, `Create preset`, collection filters,
refresh, and batch operations never move to the Detail action row.

### Global actions

Configuration, Migration, Review Recovery, and cross-page navigation remain in
the navigation rail or `Ctrl+P`. The footer shows shortcuts valid for the
currently focused pane; it is not a third action bar.

Each pane shows no more than three primary actions. Additional actions use a
pane-owned `More` menu. Confirmation and input overlays continue to expose only
their own `Confirm/Apply` and `Cancel` actions.

## Focus and input behavior

The focus order on a wide screen is:

```text
Navigation -> Collection -> Collection actions -> Detail -> Detail actions
```

Unavailable or empty regions are skipped. `Tab` and `Shift+Tab` follow this
order. Enter executes the focused semantic action; Escape returns one level or
does nothing at a root page. Mouse hit regions are generated from the same two
action-bar layouts as rendering, so a Collection action can never dispatch as
a Detail action or vice versa.

On compact and narrow layouts, only the active pane action set is rendered:

- a collection page shows Collection actions;
- an opened detail page shows Detail actions; and
- back restores the collection with its previous cursor, scroll, selection,
  and action focus.

Keyboard shortcuts and `Ctrl+P` invoke the same semantic dispatcher used by
visible buttons. Busy mutations reject all duplicate action submission paths.

## Workspace route behavior

### Global

The Collection pane shows global targets or their effective binding
projection. Collection actions cover selection and sync/refresh operations.
Detail actions operate on the selected exact global target or binding.

### Agents

The Collection pane lists supported agents. Opening an agent shows its exact
skill projection. Creating projects or changing unrelated global state is not
offered from an agent Detail action row.

### Projects

The Collection pane lists registered projects and always owns
`Create project`. A selected project Detail pane owns `Add skill`,
`Apply preset`, and `More`; `More` contains rename, manage agents, change
directory, sync preview, and remove project. Opening Common or an agent scope
preserves the exact project/scope breadcrumb and existing typed binding plans.

Project creation remains path-first and does not reintroduce encoded
pipe-separated input.

## State preservation

Changing focus between the two action rows must not change the selected row.
Opening and closing Detail preserves Collection cursor, scroll, filters, and
multi-selection. Direct navigation among Global, Agents, and Projects preserves
independent cursor/scroll state per route for the lifetime of the model.

Incremental inventory and snapshot refresh restore active rows by stable key.
If a selected item disappears, focus clamps to the nearest surviving row and
the Detail action row becomes unavailable until a new item is selected.

## Safety and error behavior

- Action placement never bypasses typed preview/confirm APIs.
- Collection batch actions operate only on explicit stable selections.
- Detail actions revalidate the active stable identity before building a
  request.
- Empty collections do not expose item actions.
- Pending recovery continues to gate every ordinary mutation.
- Partial results, warnings, conflicts, paths, and underlying errors remain
  visible in the owning pane or confirmation overlay.

## Verification

Implementation must add tests for:

1. the grouped navigation order and non-interactive section headings;
2. direct Global, Agents, and Projects routing by keyboard, mouse, launch alias,
   and command palette;
3. absence of the former Workspaces landing collection;
4. exact Collection-versus-Detail action ownership for Library, Global,
   Agents, Projects, Presets, Status, and Migration;
5. empty Library/Projects/Presets collection actions;
6. wide focus order and compact/narrow action-set switching;
7. keyboard/mouse parity and render/hit geometry for both action rows;
8. cursor, scroll, filter, selection, and per-route state preservation;
9. no mutation from selection, navigation, focus changes, or cancellation; and
10. full package, race, vet, native build, Windows build, and end-to-end
    regression verification.
