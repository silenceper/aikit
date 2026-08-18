# Activity Status Bar Design

Date: 2026-08-18
Status: Approved
Baseline: `c76fc1c`

## Goal

Replace the generic visible `busy` label with a compact, semantic activity
status that tells the user what aikit is doing, whether the operation is
read-only or mutating, whether real progress is available, and how to inspect
the result.

This design changes TUI presentation and interaction gating only. It does not
change app requests, preview/confirmation boundaries, recovery semantics, or
filesystem mutation behavior.

## Activity model

The model gains one structured activity value with these fields:

- kind: reading, network, mutating, success, warning, or error;
- label: a concise user-facing phase such as `Checking updates`;
- current and total: optional real progress counters;
- item: an optional current skill, project, or root label;
- review target: an optional typed detail kind plus stable item/operation key;
- generation: an identity used to ignore stale timer and progress messages.

Existing `MutationBusy` remains the authoritative mutation safety gate.
Existing `Busy` remains a command-in-flight and duplicate-submission guard, but
it is no longer a blanket keyboard and mouse gate. Rendering must never infer a
user-facing phase from either boolean. Commands set an explicit activity when
they start and replace it with a typed success, warning, or error when they
finish.

Progress is shown only when the backend already provides truthful current and
total values. Unknown progress uses a spinner and phase label; the TUI never
invents a percentage.

## Interaction behavior

Read-only work includes local inventory, detail loading, configuration
validation/reload, comparison, previews, and update checks. While read-only
work is active:

- all action submission is blocked, including starting a different preview or
  network request;
- view navigation, section/list/detail scrolling, loaded detail toggle,
  filtering, help, status focus, and normal cancellation remain usable;
- `Ctrl+Q` still exits normally; and
- stale asynchronous results remain guarded by the existing generation or
  request identity rules.

The input router classifies semantic UI actions as browsing or submitting.
Keyboard and mouse call the same classifier. This avoids maintaining a fragile
list of individual keys in separate handlers.

Mutating work includes confirmed config, library, link, preset, project,
recovery, sync, update, import, and adopt operations. While mutating work is
active, existing keyboard and mouse mutation gates remain in force. The status
explicitly says which mutation is running instead of displaying `busy`.

Successful activity is visible for exactly three seconds and then
returns to the ordinary contextual hint. Warning and error results persist
until the user opens the result, explicitly dismisses it, or starts another
operation.

A reviewable activity carries a `ReviewTarget` containing a closed detail kind
(`full-error`, `batch-result`, `operation-result`, `recovery-result`,
`status-item`, `update-check`, or `inventory-issue`) and the stable key or
operation ID required by that detail. It never stores an arbitrary callback.
Review activation uses only this target and the already-loaded typed model.

The status bar becomes a conditional `FocusStatus` target when a review target
exists. It does not steal focus on completion. `Tab` can reach it, mouse click
focuses it, and Enter opens the exact typed result while `FocusStatus` is
active. Ordinary Enter continues to operate on the active row or action. Before
status focus, the hint says `Tab or click to review`; while focused it says
`Enter to review`.

## Status-bar layout

The footer remains one compact line inside the existing Shortcuts panel:

```text
<activity>  |  <contextual shortcuts>
```

At 60 columns and wider, activity owns the left side and shortcuts use the
remaining cells. Below 60 columns, activity and real progress take priority;
secondary shortcuts are clipped, while `Ctrl+Q` remains reachable when space
allows. All clipping uses display-cell width and preserves grapheme clusters.

The app bar may show only a short semantic phase (`scanning`, `checking`, or
`applying`). It must not repeat the full footer sentence and must never render
the literal word `busy`.

## Semantic colors and non-color fallback

Colors are used only for the leading state marker and phase, not as a full-width
background:

- blue spinner: local/read-only work;
- cyan spinner: explicit network work;
- purple solid marker: mutating work;
- green check: success;
- yellow exclamation: warning or partial result; and
- red cross: failure.

Examples:

```text
⠋ Scanning local skills · 3/6
⠋ Checking updates · 12/33
● Applying preset · 2/5
✓ Added 3 skills
! Completed with 2 issues · Tab or click to review
× Update failed · Tab or click for details
```

The adaptive light/dark theme supplies the concrete colors. Reduced-color and
`NO_COLOR` modes retain the non-color markers and wording, so meaning never
depends on color. Spinner animation may use a reduced frame set when motion is
disabled.

## Event and timer flow

Bubble Tea commands continue to perform all asynchronous work. A lightweight
spinner tick updates only the activity frame while an indeterminate activity
is current. Each tick carries the activity generation; stale ticks are ignored.

Inventory events update real root progress. Other commands without progress
events remain indeterminate. Completion creates a new generation and schedules
a success-expiry message. Expiry clears only the same success generation, so a
late timer cannot erase a newer warning, failure, or operation.

Opening a warning/error review acknowledges and clears the activity banner;
closing the typed detail does not recreate it. The authoritative issue remains
available in its existing Overview, Status, batch, recovery, or error detail
surface. Explicit dismissal clears only the banner and never deletes typed
result data.

The activity is a display state, not a second source of truth for business
results. Typed `Snapshot`, `Result`, `BatchResult`, `RecoveryResult`, warnings,
issues, and errors remain authoritative and continue to populate existing
detail views.

## Error and cancellation behavior

- A top-level error and typed per-item issues remain visible together.
- Partial completion uses warning state and never claims full success.
- Starting a new operation replaces an old transient success.
- Canceling a preview returns to the previous route and emits a neutral
  `Cancelled; no changes made` status.
- Emergency `Ctrl+C` behavior remains unchanged.
- Read-only activity cancellation must not leave the interface permanently
  gated or allow a stale completion to replace the current activity.

## Verification

Implementation must prove:

1. the visible word `busy` no longer appears in app-bar or footer states;
2. reading, network, mutating, success, warning, and error render distinct
   markers and semantic colors;
3. `NO_COLOR` and reduced modes retain unambiguous markers;
4. known progress displays exact current/total and unknown progress displays
   no fake percentage;
5. read-only activity permits navigation, scrolling, filtering, help, status
   focus, cancellation, and normal quit while rejecting every new submission;
6. mutating activity retains the existing keyboard and mouse safety gate;
7. success expires only if its generation is still current;
8. warning/error persists; Tab/click focuses the status target, and Enter then
   opens only the exact typed detail identified by `ReviewTarget`;
9. stale spinner, timer, inventory, and command messages cannot overwrite a
   newer activity;
10. status and shortcut content stay within 24, 38, 80, and 120 columns with
    CJK and emoji text;
11. keyboard and mouse behavior remains equivalent; and
12. package, full, race, vet, native, Windows, E2E, and real-config read-only
    verification pass.
