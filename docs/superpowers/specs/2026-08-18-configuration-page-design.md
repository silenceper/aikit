# Configuration Page Design

## Goal

Replace the blocking Configuration modal with a normal read-only tool page. Users must be able to inspect configuration and run its safe actions without losing access to the primary navigation.

## Interaction model

- Selecting `Tools / Configuration` activates a normal page state, not an overlay.
- The left navigation remains rendered, focused, clickable, and keyboard reachable while Configuration is active.
- Configuration owns the main collection/detail area just like the other top-level destinations.
- `Esc` follows normal page navigation. It closes an actual child detail first, then returns focus/back normally; it does not dismiss an invisible modal layer.
- `Ctrl+X` remains the only application quit chord.

## Page content

The page shows a concise configuration summary:

- config file path;
- library directory;
- cache directory;
- last validation state and message, when available.

The page action bar exposes only the existing read-only operations:

- `Validate`;
- `Reload`;
- `Show paths`.

`Show paths` may open the existing text-detail overlay because it is a child detail with an explicit `Close` action. Validation and reload results remain visible in the ordinary status/activity area. Configuration does not expose editor or clipboard actions.

## State and routing

- Add a dedicated normal-page representation for Configuration rather than using `ModeConfiguration` as an overlay mode.
- Navigation activation loads `ConfigurationDetail` asynchronously while preserving normal page routing.
- `hasOverlay` must return false for the Configuration page.
- Confirm, Input, Error detail, Help, Command, More, and other genuine transient surfaces remain overlays and continue to capture input.
- Configuration actions use the existing typed `app.Service` methods and do not mutate configuration.

## Responsive behavior

- Wide and compact layouts render Configuration inside the framed main workspace, with actions in the page-owned action bar.
- Narrow layouts retain breadcrumb/back behavior and the existing paged action-bar geometry, including pure-mouse access to every action.
- The page must not grow beyond the terminal bounds; long paths are clipped/wrapped using display-cell-aware helpers and remain available through `Show paths`.

## Keyboard and mouse parity

- Navigation click and navigation keyboard activation produce the same Configuration page state and request.
- `Tab`, arrow keys, and `Enter` can reach and execute every page action.
- Mouse clicks use the same action registry and geometry as keyboard activation.
- Clicking another navigation destination while Configuration is open switches immediately; no explicit close is required.
- Busy read operations block duplicate action submission but do not turn Configuration into a modal or prevent navigation after the operation completes.

## Error handling

- Configuration load, validation, and reload errors are retained in status/activity review.
- An error does not trap the user on Configuration.
- `Show paths` and error details are closable child overlays and return to Configuration with the previous page focus restored.

## Acceptance tests

1. Opening Configuration yields a normal page and `hasOverlay()` is false.
2. With Configuration active, mouse and keyboard can switch to Library, Presets, Global, Agents, Projects, and Overview.
3. Validate, Reload, and Show paths remain keyboard/mouse equivalent and single-submit while busy.
4. Show paths closes back to Configuration; Configuration itself requires no Close action.
5. Wide, compact, narrow, and low-height rendering stays within bounds and preserves action reachability.
6. Existing modal capture tests continue to pass for real overlays.
7. Full TUI, repository, race, vet, native, Windows, Linux, and E2E verification passes.

## Non-goals

- Editing the YAML in the TUI.
- Clipboard integration.
- Launching `$EDITOR`.
- Changing configuration paths or backend APIs.
- Redesigning unrelated pages or overlays.
