# Configuration Page Design

## Goal

Replace the blocking Configuration modal with a normal read-only tool page. Users must be able to inspect configuration and run its safe actions without losing access to the primary navigation.

## Interaction model

- Selecting `Tools / Configuration` activates a normal page state, not an overlay.
- The left navigation remains rendered, focused, clickable, and keyboard reachable while Configuration is active.
- Configuration owns the main collection/detail area just like the other top-level destinations.
- `Esc` follows normal page navigation. It closes an actual child detail first, then returns focus/back normally; it does not dismiss an invisible modal layer.
- `Ctrl+Q` remains the normal application quit chord; emergency `Ctrl+C` behavior is unchanged.

## Page content

The page shows a concise configuration summary:

- config file path;
- library directory;
- cache directory;
- validation state (`Not validated`, `Valid`, or `Invalid`) and the last validation message, when available.

The page action bar exposes only the existing read-only operations:

- `Validate`;
- `Reload`;
- `Show paths`.

`Show paths` may open the existing text-detail overlay because it is a child detail with an explicit `Close` action. Validation and reload results remain visible in the ordinary status/activity area. Configuration does not expose editor or clipboard actions.

## State and routing

- Add a dedicated normal-page representation for Configuration rather than using `ModeConfiguration` as an overlay mode.
- Navigation activation loads `ConfigurationDetail` asynchronously while preserving normal page routing.
- `hasOverlay` must return false for the Configuration page.
- Keep a TUI-local validation display state containing `Attempted`, `Valid`, and `Message`. A successful or failed validation command updates this state; unrelated status/activity updates do not erase it. Leaving and returning to Configuration preserves it for the lifetime of the TUI session.
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
- Busy Configuration reads block duplicate Configuration action submission, but navigation remains keyboard- and mouse-operable while load, validation, or reload is in flight.
- Navigating away does not submit another Configuration action. A late Configuration result may update cached Configuration state and activity history, but must not switch the active route back to Configuration.

## Error handling

- Configuration load, validation, and reload errors are retained in status/activity review.
- An error does not trap the user on Configuration.
- `Show paths` and error details are closable child overlays and return to Configuration with the previous page focus restored.

## Acceptance tests

1. Opening Configuration yields a normal page and `hasOverlay()` is false.
2. With Configuration active, mouse and keyboard can switch to Library, Presets, Global, Agents, Projects, and Overview.
3. During Configuration load, Validate, and Reload, keyboard and mouse can navigate away; late results do not change the active route.
4. Validate, Reload, and Show paths remain keyboard/mouse equivalent and single-submit while busy.
5. Validation initially renders `Not validated`; success renders `Valid`; failure renders `Invalid` plus its message; leaving and returning preserves the result.
6. Show paths closes back to Configuration; Configuration itself requires no Close action.
7. Wide, compact, narrow, and low-height rendering stays within bounds and preserves action reachability.
8. Existing modal capture tests continue to pass for real overlays.
9. Full TUI, repository, race, vet, native, Windows, Linux, and E2E verification passes.

## Non-goals

- Editing the YAML in the TUI.
- Clipboard integration.
- Launching `$EDITOR`.
- Changing configuration paths or backend APIs.
- Redesigning unrelated pages or overlays.
