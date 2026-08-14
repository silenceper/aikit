# Modern TUI Visual Design

Date: 2026-08-14
Status: Approved for implementation

## Goal

Replace the current verification-oriented presentation with an English-first,
calm, modern skills-manager interface. Preserve every existing workflow and
typed preview/mutation boundary. This change owns presentation, layout, focus,
and interaction polish only.

## Visual direction

- Use the terminal's existing background and foreground as the base so the UI
  works in both light and dark terminals.
- Use a restrained blue-violet accent for the active navigation item, active
  row, focus ring, and primary action.
- Reserve green, amber, and red for success, warning, and error states.
- Every focus and state must also have a non-color signal: a marker, label,
  weight, or border. Respect `NO_COLOR`; when terminal background or color
  capability is unknown, fall back to terminal defaults plus ASCII markers and
  bold/underline. The interface must remain unambiguous in 8/16-color output.
- Avoid large filled backgrounds, dense borders, and simultaneous highlights.
- Prefer whitespace, alignment, concise labels, and one obvious focal point.

## Structural dividers

Use a light k9s-inspired divider system to clarify large functional regions
without turning every item into a boxed panel:

- Render one horizontal divider below the app bar.
- At widths 60 and above, render one vertical divider between navigation and
  the main workspace. The segment beside the active navigation item uses the
  blue-violet accent while the remaining divider stays muted.
- Render one horizontal divider above the status and shortcut area.
- Render confirmation, input, configuration, More, recovery, and error-detail
  overlays with one complete thin border. The title is integrated into the top
  border and the primary/cancel actions stay inside the bottom border.
- Render detail groups with short local dividers such as `- Summary`,
  `- Usage`, `- Files`, and `- Diagnostics`; do not extend these rules across
  the full workspace.
- Do not draw boxes around metrics, rows, badges, or individual buttons. Those
  continue to use whitespace, a slim selection marker, and semantic labels.

Divider characters come from one semantic glyph set. Unicode-capable output
uses light box-drawing glyphs. `NO_COLOR`, reduced-capability, and explicit
ASCII fallback modes use `-`, `|`, and `+`. Focus and region boundaries must
remain understandable when ANSI styling is stripped.

## Responsive shell

At 96 columns and above, render a vertical navigation rail on the left and one
main workspace on the right. The rail contains Overview, Library, Workspaces,
Presets, Migration, and Status with a compact count or attention indicator.

At 60-95 columns, collapse the rail to short labels while preserving the same
navigation and hit targets. Show details as an overlay or focused page rather
than forcing a narrow split pane.

Below 60 columns, render a single pane with a breadcrumb and a Back action. The
active collection, details, and modal actions must remain reachable with both
keyboard and mouse. The navigation divider disappears in this mode; only the
app-bar and footer dividers remain. The breadcrumb does not add another line.

The top app bar contains only `aikit`, the current context, and scan/busy
status. The bottom bar shows only actions valid for the current focus.

## Overview

Overview is attention-first. Render four compact metrics: Skills, Updates,
Unmanaged, and Issues. Below them, render `Needs attention` items sorted by
severity. Each item has a small state marker, a concise title, one muted line
of context, and one recommended action. Healthy empty state is calm and short.

The Updates metric uses only update data already present in the offline startup
snapshot. The TUI does not read or parse the update checker's private cache.
When the snapshot contains no update data, render `Not checked`; only an
explicit Refresh/Check action may use the network.

Attention severity is `recovery/error > conflict/drift > unmanaged/update >
informational`. Within a severity, order by the existing stable item key. While
incremental inventory events arrive, preserve the selected key and scroll
anchor; a selected item may move only after a deliberate user refresh or when
the selected item disappears.

Heavy inventory details, paths, hashes, file trees, and diagnostics are not
shown on the first screen.

## Collections and details

Library, workspace, preset, migration, and status collections use a two-line
item treatment instead of a dense table:

1. primary line: name and state badge;
2. secondary line: source, scope, or concise context in muted text.

The selected item uses a slim accent marker and stronger text, not a full-width
inverted background. Status badges have consistent width and semantic color.

Details are grouped as Summary, Usage, Files, and Diagnostics. Summary is
visible first. Additional groups appear only when relevant or explicitly
opened. The detail action bar shows no more than three primary actions; all
other actions remain under More.

## Interaction

- Keyboard and mouse continue to call the same action dispatcher.
- The focused navigation item, list item, detail section, and action are
  visually distinct; only one receives the strongest focus treatment.
- Busy operations show a compact spinner plus the current operation. Existing
  content remains visible and duplicate mutations stay blocked.
- Confirmation and input overlays use a centered, bounded panel with a clear
  title, summary, scroll position when needed, and one emphasized action.
- Errors remain next to their owning item when possible. The global status line
  is limited to one concise sentence.
- At heights 8 and 12, overlays keep their title and primary/cancel actions
  fixed and scroll only the body. Confirm and Cancel must always remain visible
  and clickable.

## Component boundaries

- `styles.go`: adaptive semantic palette and reusable text/badge/panel styles.
- `layout.go`: pure responsive shell, navigation, content, detail, overlay, and
  footer rectangles.
- A shared divider/panel primitive owns glyph selection and exact line
  geometry; shell rendering, overlay rendering, and mouse hit testing consume
  that result rather than recalculating border positions.
- `render.go`: shell and content composition only.
- `rows.go`: semantic presentation fields for two-line collection items.
- `input_mouse.go`: hit testing based exclusively on layout/render geometry.
- Business requests and app/migration service interfaces remain unchanged.

## Testing and acceptance

- Semantic render tests cover 120, 96, 95, 80, 60, 59, 38, and 24 columns and
  heights 8, 12, 16, and 30.
- Layout rectangles never overlap or escape terminal bounds.
- Overview contains the four metrics and attention ordering without heavy
  details.
- Empty, healthy, loading, partial, warning, and error states remain readable.
- Keyboard/mouse parity remains valid after navigation and action placement
  changes.
- Divider tests cover wide, compact, narrow, height 8, and height 12 layouts.
  Lines never overwrite labels, row content, scroll indicators, or actions.
- Overlay borders and their title/action slots remain completely visible and
  clickable at supported minimum sizes.
- Unicode and ASCII glyph profiles produce equivalent region boundaries after
  ANSI stripping; CJK and emoji content remains within terminal cell bounds.
- ANSI output is visually inspected in dark and light palette assumptions and
  semantically tested with `NO_COLOR` and a reduced 8/16-color fallback. Focus
  and severity assertions must pass after ANSI color is stripped.
- Full Go tests, race tests, vet, native/Windows builds, E2E, and diff checks
  must remain green.

## Non-goals

- No new business workflow or backend API.
- No Web UI or image assets.
- No fixed terminal theme, gradients, hover dependency, or Unicode-only
  controls that fail in ordinary terminals.
- No automatic mutation during startup scan.
- No startup network request; update counts are cached or `Not checked` until
  the user explicitly requests a check.
