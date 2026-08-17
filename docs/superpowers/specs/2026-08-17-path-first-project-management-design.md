# Path-first Project Management Design

Date: 2026-08-17
Status: Approved interaction design; pending implementation review

## Goal

Make project registration and project-scoped skill management feel like a
normal interactive application instead of editing a compact configuration
syntax. Creating a project asks only for its directory. The TUI derives safe
defaults, presents an exact preview, and then exposes project name, agents,
common skills, agent-specific skills, and presets as separate operations.

The workflow must never ask users to enter pipe-separated or comma-encoded
records such as `name|path|agents`.

## Chosen approach

Use a path-first registration flow followed by a dedicated project detail
page. This is preferred over a creation wizard because the common case remains
one input plus one confirmation, and preferred over a structured single form
because each later operation has its own validation, preview, and cancellation
boundary.

## Project registration

`Create project` opens one input named `Project directory`.

After submission, the application layer performs a strictly read-only
registration preview:

- canonicalize the supplied path and require an existing directory;
- derive the default project name from the directory basename;
- reject an already registered canonical path;
- detect configured agents from their known project integration directories;
- never select every agent merely because no agent was detected;
- validate the derived project against a cloned configuration; and
- return the exact name, canonical path, detected agents, warnings, and planned
  link actions to the TUI.

Directory validation is shared by preview and mutation. Immediately before a
new project is checkpointed, and immediately before an existing project path is
changed, the application reopens and revalidates the path as an existing,
readable directory, derives the same canonical location, and compares its
platform file identity with the opaque identity returned by the preview. A
different object at the same path or a symlink retarget fails closed. This
closes the preview-to-confirm replacement window. It intentionally applies to CLI
`project add` and `project edit --path` as well as the TUI; CLI flags and output
shape remain unchanged, but missing or non-directory project paths are rejected
instead of being stored for possible future creation.

Agent detection iterates `agent.All()` in registry order and recognizes the
five registered agents: `cursor`, `claude-code`, `codex`, `copilot`, and
`windsurf`. For each item it checks the integration directory returned by
`ProjectSkillDir`. A real directory selects the agent. A missing directory is
not selected. A symlink/reparse point, non-directory, unreadable entry, or other
inspection error is not selected and produces a warning containing that agent
and path. Detection never follows such an entry outside the authorized project
root.

Directory identity and reparse detection are platform-specific. Darwin/Linux
use device and inode identity. Windows uses volume serial plus file index and
explicitly checks `FILE_ATTRIBUTE_REPARSE_POINT`; `os.FileMode` alone is not an
adequate Windows reparse-point test. Unsupported platforms fail closed instead
of returning a weak timestamp/size identity.

The confirmation panel shows the project name, canonical path, detected agents
or `None detected`, warnings, and every planned filesystem action. Cancel makes
no change. Confirm calls the existing project mutation with those exact values.
After success the TUI refreshes its snapshot and opens the newly registered
project.

If the derived name is invalid or already used, the typed preview returns
`NeedsName` and a typed `NameIssue` code instead of a generic error. The TUI
then asks for a project name in a separate input and rebuilds the preview. It
must not parse validation error strings or duplicate config validation rules.
It does not fall back to an encoded multi-field input. A project with no
detected agent is valid and can be configured from its detail page.

## Application contract

Add a typed, read-only preview contract owned by `internal/app`, conceptually:

```go
type ProjectRegistrationRequest struct {
    Path string
    Name string // optional conflict override
}

type ProjectRegistrationPreview struct {
    Name     string
    Path     string
    PathIdentity string
    Agents   []string
    Preview  ProjectEditPreview
    Warnings []string
    NeedsName bool
    NameIssue ProjectNameIssue
}
```

`ProjectEditPreview` carries the same opaque `PathIdentity` whenever creation
or a path change is previewed. The confirmed `ProjectEditRequest` carries it as
`ExpectedPathIdentity`. TUI confirmations must copy that exact value; the app
rejects a mismatch before config or filesystem mutation. Direct CLI requests
do not have a prior preview token, but still perform existing-directory and
canonical-path validation at the mutation boundary.

`ProjectNameIssue` is a small closed enum that distinguishes at least an
invalid derived/overridden name from a duplicate project name. Path,
permission, detection, and other failures remain ordinary typed method errors
or warnings as specified above. When `NeedsName` is true, the method returns no
mutation plan and the frontend may only cancel or provide a new name.

`Service.PreviewProjectRegistration` must use only configuration loading and
filesystem inspection. It must not take a mutation lock, recover pending work,
checkpoint configuration, create directories, or execute link actions. The
existing `PreviewProjectEdit` and `EditProject` remain authoritative for the
actual configuration and link plan; registration preview should reuse their
validation/planning logic rather than duplicate it in the TUI.

All frontends and test fakes that implement `app.Service` receive the typed
method. The CLI remains compatible with `aikit project add` and does not adopt
the TUI-specific interaction state.

## Project detail interaction

Opening a registered project enters a project detail collection rather than a
generic text editor. The header shows project name, canonical path, and active
agents. The collection contains:

- `Common Skills`, representing the binding inherited by every declared agent;
- one row for each configured agent, representing that agent's project binding;
- clear empty-state guidance when the project currently has no agents.

Opening `Common Skills` or an agent row uses the existing library skill list.
Space or the visible Enable/Disable action toggles a skill through the typed
binding preview and confirmation flow already used elsewhere in the TUI.

Project management remains progressively disclosed. The primary page exposes
at most three actions; a `Manage` or `More` panel contains:

- `Rename` — one project-name input;
- `Manage agents` — a checkbox list for all five supported agents;
- `Change path` — one directory input with exact old-path cleanup and new-path
  link preview;
- `Apply preset` — choose a preset, then choose `Common` or one configured
  agent as the exact target;
- `Remove project` — retain the existing exact cleanup preview and explicit
  confirmation; and
- `Close`.

Saving `Manage agents` computes additions and removals from the current project
snapshot and submits one `ProjectEditRequest`. Removing an agent must show its
managed-link cleanup plan and preserve unknown content. Adding an agent must
show the links that will be created. Cancelling any input, checklist, or
confirmation produces zero mutations.

Rename, path change, agent management, and preset application are independent
operations. They are not concatenated into one mini-language and do not require
users to remember delimiters.

## Keyboard and mouse behavior

Every visible project action is reachable with both keyboard and mouse through
the shared action registry and shared hit geometry:

- Enter or click opens the selected project, scope, or action;
- Space or click toggles an agent/skill checkbox;
- Tab and arrow keys move between list, details, and action controls;
- Escape returns to the exact prior project context or cancels a preview;
- Busy state rejects duplicate submissions; and
- narrow action bars retain mouse-only pagination to every action.

After a snapshot refresh or project edit, selection is restored by stable
project/scope identity rather than row index. If rename or agent removal makes
that identity disappear, selection falls back deterministically to the renamed
project, then `Common Skills`; if the whole project was removed, it falls back
to the project collection.

## Error handling and safety

- Missing paths, non-directories, permission failures, invalid names, duplicate
  names, and duplicate canonical paths are reported before mutation.
- Detection failures for one optional agent directory are visible warnings and
  do not silently select that agent.
- Path changes and agent removals always require an exact cleanup preview.
- Unknown project content and unrecognized links are never removed.
- Pending recovery continues to gate mutations through the existing app
  contract; registration preview itself remains read-only.
- Result and link issues remain visible as structured paths and errors instead
  of being flattened into a generic success or failure message.

## Testing

Use strict RED/GREEN cycles for:

1. a path-only create input replacing the three-field parser;
2. basename defaulting, canonical path validation, duplicate-path rejection,
   and zero-write preview behavior;
3. detection of each supported agent, no-agent behavior, and symlink/reparse
   fail-closed behavior;
4. exact preview to confirmed `EditProject` request parity and cancel zero-write;
5. successful creation routing to the stable project detail identity;
6. rename, path change, agent add/remove, and empty-project interaction;
7. project Common and per-agent skill toggles;
8. preset selection followed by an exact Common or per-agent target;
9. keyboard/mouse parity, busy gating, narrow layout, and stable selection; and
10. partial/error results retaining exact warnings, paths, and underlying
    errors.

Focused `internal/app`, `internal/tui`, and `cmd` tests must pass, followed by
the full test suite, race tests, vet, native build, Windows cross-build, E2E,
formatting, and diff checks.

## Non-goals

- No new project-scoped storage model; existing `config.Project` bindings stay
  authoritative.
- No automatic selection of all agents.
- No implicit deletion or adoption of existing project content.
- No change to CLI flags or machine-readable output.
- No redesign of Library, Migration, Status, or Configuration outside the
  project navigation needed for this workflow.
