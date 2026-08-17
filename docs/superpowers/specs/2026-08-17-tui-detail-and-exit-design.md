# TUI Detail Boundaries and Safe Exit

## Problem

The Library detail pane currently appends every discovered file and the entire
`SKILL.md` document to the main page. Real skills contain 149–223 lines, so the
page is technically clipped but still behaves like an unbounded document and
pushes routine actions out of the user's mental focus.

The top-level keyboard contract also makes accidental exit too easy: `q` exits
from a normal page and `Esc` can be confused with quitting rather than going
back.

## Interaction design

### Library details

- Opening a Library skill keeps the main detail pane concise.
- The pane has a hard display-line budget derived from the available detail
  body after fixed title and actions. Summary is always first; Usage and Files
  each consume at most three wrapped display lines; the content preview uses
  at most six wrapped display lines and never exceeds the remaining budget.
- Omitted usage locations, files, source lines, and backend truncation are
  represented by explicit counts or markers, never silent truncation.
- A `View SKILL.md` action opens the already-loaded `SkillMD` preview (up to the
  backend's existing 64 KiB safety limit) in a bounded, scrollable
  overlay with a fixed title, Close action, keyboard scrolling, and mouse-wheel
  scrolling.
- If the backend marked the preview truncated, the overlay ends with an
  explicit `preview truncated at 64 KiB` marker; it never claims to show the
  complete file.
- Returning from the overlay restores the same selected skill and detail
  scroll position.
- Rendered lines stay within the shared terminal layout at 24, 38, 59, 80, and
  120 columns.

### Exit and back navigation

- `Esc` cancels the current modal, closes a narrow/full detail view, or returns
  Actions/Detail focus to the collection. At the top-level collection with
  List focus it is a no-op.
- Plain `q` never exits from a page or non-text modal and displays a short hint
  to use `Ctrl+Q`. In Filter/Input mode it remains ordinary input text.
- `Ctrl+Q` is the only normal quit command from pages, details, and modals. It
  may cancel read-only background loading, but it is refused while
  `MutationBusy` is true so a write cannot be abandoned mid-protocol.
- `Ctrl+C` remains an emergency quit command, including while an operation is
  stuck. It is labeled as emergency behavior rather than the primary shortcut.
- Footer and keyboard help text use the same shortcut contract.

## Safety and compatibility

- Viewing full content is read-only and performs no backend call after the
  typed `SkillDetail` has loaded.
- Complete content remains bounded by the existing overlay geometry; only the
  overlay body scrolls.
- Mouse and keyboard use the same `View SKILL.md` and Close action dispatcher.
- No app, library, configuration, or migration API changes are required.

## Verification

- Regression test with a real-sized 200+ line `SKILL.md` proves the main detail
  obeys its wrapped display-line budget and the final loaded line is reachable
  in the overlay; a separate case proves the 64 KiB marker is visible.
- Width/height matrix proves no rendered line or screen exceeds its bounds.
- A table-driven state matrix covers List/Actions/Detail/Filter/Input/Confirm/
  More/ErrorDetail with `Esc`, `q`, and `Ctrl+Q`, plus MutationBusy refusal and
  emergency `Ctrl+C`.
- Keyboard/mouse parity tests cover opening, scrolling, and closing full skill
  content.
