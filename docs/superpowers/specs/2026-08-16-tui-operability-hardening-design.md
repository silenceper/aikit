# TUI Operability Hardening Design

Date: 2026-08-16
Status: Approved for implementation

## Goal

Make the current Bubble Tea TUI predictable, safe, and pleasant in both wide
and narrow terminals without changing application-service mutation boundaries.
The quality bar is that visible actions work for the current item, cancellation
restores the exact prior context, and users can identify what they are about to
change before confirming it.

## Interaction model

- Overview attention rows carry an explicit destination and open the matching
  Migration, Status, Updates, or Recovery context.
- Confirmation overlays remember their parent mode, focus, cursor, scroll, and
  selection. Cancel restores that state exactly and never falls into a temporary
  preview collection.
- Search edits a draft value. Apply commits it; Cancel preserves the previous
  filter. Active filters and result counts stay visible outside the overlay.
- Primary and More actions are derived from the selected row and current
  selection. Invalid actions are hidden rather than shown and rejected later.
- `q` quits from a normal page. `Esc` owns focus/back/cancel behavior.

## Information design

- Migration rows show scope/origin and recommended action on the secondary
  line. Duplicate names remain distinguishable, and broken links receive a
  human-readable label instead of an opaque inventory key.
- Collection headings show active filters and result counts. Long collections
  show the visible range and total count.
- Informational counts are rendered as metadata, not warning-like `[INFO]`
  badges. Empty states include a next action when one exists.
- Wide terminals use a readable bounded content width and a contextual detail
  area; compact terminals retain the navigation rail; narrow terminals retain
  every critical metric and action through wrapping or pagination.

## Safe previews

Confirmation body lines wrap to the panel width before vertical pagination.
Paths and conflicts are never ellipsized inside a mutation confirmation. The
title and Confirm/Cancel action row stay fixed at small heights.

## Configuration and help

Configuration remains read-only and exposes Validate, Reload, Show Paths, and
Close. This hardening pass does not add clipboard or editor process authority.
Help documents Tab/Shift+Tab, arrows, filtering, clearing a draft, paging,
mouse, Esc, and q accurately.

## Testing

Add regression tests for:

- every Overview attention destination;
- cancel restoration after startup-inventory migration preview;
- search draft/apply/cancel and visible filter state;
- contextual action availability;
- duplicate migration identity and human-readable broken links;
- wrapped exact paths at 38 columns;
- responsive metrics, range indicators, empty-state guidance, and q behavior;
- keyboard/mouse parity after action filtering.

Existing race tests, vet, E2E, responsive render matrices, `NO_COLOR`, and
Windows build checks remain green.
