# Library Multi-select Action Bar Design

Date: 2026-08-18
Status: Approved by user
Baseline: `0e04dd3`

## Goal

Make Library multi-selection visibly useful and immediately operable. Selecting
one or more skills replaces the ordinary collection actions with a compact,
contextual batch-action bar. The interaction follows the existing K9s-inspired
shell without adding another row, increasing information density, or weakening
the existing preview, confirmation, and recovery guarantees.

This is a TUI-only change. It reuses the existing app batch, preview, scope
selection, confirmation, and semantic activity APIs. It does not change config,
library, link, or migration business behavior.

## Interaction model

The normal Library collection action row remains unchanged while nothing is
selected. The first selected skill switches that same row to:

```text
2 selected  Enable  Disable  Update  Remove  Clear
```

The action row replaces the existing `Add source / More` row; it never adds
height or moves the list and detail panes. The count is informational and is not
a separate focus target.

Keyboard mnemonic letters are integrated into the English action words instead
of being rendered as separate shortcut blocks. The mnemonics are `E`, `D`, `U`,
`R`, and `C` for Enable, Disable, Update, Remove, and Clear. Color-capable themes
highlight the mnemonic character. Reduced-color and `NO_COLOR` modes use
underline and the existing focus marker, so discoverability never depends on
color alone. The complete displayed label remains one mouse hit target.

In Library table mode with a non-empty selection and no overlay, input, filter,
or other modal state active, these mnemonic keys immediately dispatch the same
typed action used by focused Enter and mouse click. They take precedence over
ordinary row shortcuts only in that state. A disabled mnemonic performs zero
backend calls and reports the same exact disabled reason as focused activation.

Space and the Library checkbox use the same selection operation. Selection is
identified by the stable skill ID rather than the visible row index. Filtering,
sorting, inventory refreshes, and asynchronous updates preserve selected skills
that still exist, including temporarily hidden selections.

## Action model

Rendering and input share one typed Library selection-action model. Each action
has:

- a stable action ID;
- a display label and mnemonic;
- an enabled flag;
- an exact disabled reason;
- the existing UI operation it dispatches; and
- enough metadata to build its preview or request without inspecting the
  rendered string.

The actions are ordered `Enable`, `Disable`, `Update`, `Remove`, and `Clear`.
Keyboard focus, mouse hit regions, responsive pagination, disabled styling, and
dispatch all consume the same ordered model. Labels are presentation only and
must not be used as dispatch identities.

Disabled actions remain visible and focusable. Activating one performs no
backend call and writes its precise reason into the semantic activity/status
line. Examples include an update selection containing a local skill, a remote
skill without a branch ref, or a skill without a complete checked remote
identity.

`Clear` is always enabled when at least one selected skill still exists.

## Batch flows

### Enable and Disable

Enable and Disable never guess a target scope. Activation opens the existing
exact scope input/choice flow. The chosen target must be one of:

- `agent:<agent>`;
- `project:<project>` for project common bindings; or
- `project-agent:<project>:<agent>`.

Every selected skill is previewed against that same exact scope. The typed
preview is shown before a single atomic `app.Batch` request is submitted.

### Update

Update is enabled only when every selected skill has a non-local supported Git
source, a non-empty branch ref, and an exact checked record for the same skill
ID whose typed state is `update-available`, whose current value equals the
ledger `Resolved`, and whose remote value is non-empty. Hidden selected skills
participate in this validation. The request copies `Current` and `Remote` from
the matching typed update-check record and copies `Ref` from the same skill's
current ledger entry into `Expected`. No update-check or backend type changes.
Any missing or mismatched field disables the whole action before `PreviewBatch`
is called.

The confirmation shows the complete batch and uses the existing exact expected
tokens. Confirmation submits one atomic batch update. If any selected skill is
ineligible, the whole action stays disabled and reports the first exact blocking
skill and reason; no partial subset is silently chosen.

### Remove

Remove makes exactly one `PreviewBatch` call with `Operation: BatchRemove`, the
complete stable selected skill-ID set, and `Force: false`. It does not fan out
per-skill `PreviewRemove` requests. The confirmation renders the returned typed
`BatchPreview`, including references, affected scopes, planned paths, warnings,
conflicts, and issues. If the preview requires Force, the first confirmation
leads to the existing second Force confirmation. Only the final confirmation
submits exactly one atomic `Batch` request with `Force: true`.

### Clear

Clear removes the Library selection locally. It never calls the backend and
returns the normal collection action row immediately.

## Responsive behavior

The action row is always one terminal line and never wraps.

- At wide widths it shows the count and all five actions.
- At medium widths it uses the existing action-bar `‹` and `›` pagination.
- At narrow widths it shows the count, the current action, and a reachable page
  control, for example `2 selected  Update  ›`.
- Keyboard Left/Right/Tab and mouse wheel/page controls use the same action
  viewport and can reach every hidden action.

The count and page controls participate in the shared render/hit geometry.
Separators and unused cells remain no-ops. Long ASCII, CJK, and emoji skill names
are clipped by display cells without changing the action row, list height,
detail boundary, or state column.

## Escape and completion behavior

Overlay and detail cancellation retain precedence. Esc first closes the active
overlay or detail surface. From the Library collection table:

1. the first Esc clears a non-empty selection and stays in Library;
2. a subsequent Esc performs the ordinary route/back behavior.

This prevents an accidental back action while the visible selection bar is
active. It does not change the existing `Ctrl+Q` quit contract.

Successful batch completion clears the selection and returns to the normal
Library actions. Failure or partial failure retains the selection so the user
can inspect issues and retry or adjust the batch. Existing mutation-busy and
semantic activity generations continue to prevent duplicate submission and
stale completion messages.

## Safety and error behavior

- Preview and confirmation remain mandatory for every mutating batch action.
- Canceling any scope, preview, Force, or confirmation surface performs zero
  mutation.
- Disabled actions perform zero backend calls.
- Typed per-item issues and aggregate errors remain reviewable and are not
  flattened into a generic message.
- An aggregate `Changed == false` result never claims an item was durably
  changed.
- Unknown or concurrently replaced content keeps the existing fail-closed app
  and recovery behavior.
- Keyboard and mouse activate the same typed action and construct the same
  request.

## Implementation boundary

Production changes are limited to `internal/tui`. The implementation should
introduce a small `librarySelectionActions` projection and adapt the existing
collection/action-bar rendering and shared input dispatch. It must reuse the
existing `app.Batch`, remove and binding previews, exact scope picker, Force
confirmation, semantic activity line, and action-bar layout primitives.

No backend API, config schema, filesystem mutation, recovery protocol, CLI
behavior, or `internal/web` code changes are part of this work.

## Verification

Implementation must prove:

1. zero, one, and multiple selections render the correct normal or contextual
   action row;
2. selection survives filters, sorting, and incremental refresh by stable ID;
3. the row remains one line and list/detail geometry is unchanged at widths 24,
   38, 59, 80, and 120, including long ASCII, CJK, and emoji names;
4. every action is reachable with keyboard and mouse at every width, including
   paged narrow layouts;
5. action text, focus, and mouse hit rectangles share one geometry source;
6. disabled actions remain visible, make zero backend calls, and expose the
   exact reason in the activity/status line;
7. Enable and Disable require an exact scope and submit one atomic batch only
   after typed preview and confirmation;
8. Update rejects the complete selection before preview if any visible or
   hidden item lacks a matching typed `update-available` record or exact token;
9. Remove makes one complete-set `PreviewBatch` call, no per-item preview calls,
   and requires the second Force confirmation when any reference exists;
10. cancel paths perform zero mutations, successful completion clears selection,
    and failure or partial failure retains it;
11. Esc closes overlay/detail first, then clears Library selection, then performs
    normal back behavior;
12. mutation-busy and activity generation guards prevent duplicate or stale
    submissions; and
13. focused TUI, full, race, vet, native build, Windows build, E2E, diff-check,
    and formatting verification pass.
