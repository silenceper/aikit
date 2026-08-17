# TUI Skills Manager Parity Audit

Date: 2026-08-17
Status: In-scope routes verified

This audit compares aikit with its approved workspace TUI specification and
uses the official `xingkongliang/skills-manager` workspace workflow as an
interaction benchmark. Feature parity means that the same local skills
management job is discoverable and safe in a terminal; it does not mean copying
the desktop UI or adopting unrelated product scope.

| Capability | Current state | Evidence / required change |
|---|---|---|
| Central library; local/Git add | Meets | Library Add source, typed preview, detail, filters, update, ref change, safe remove |
| Automatic local inventory | Meets | Offline startup Snapshot plus incremental all-project Inventory |
| Exact import/adopt/link/compare | Meets | Migration exact selectors, preview, confirmation, session Ignore |
| Global per-agent workspace | Meets | Workspaces > Agents > agent lists Library skills and previews toggles |
| All Agents global workspace | Meets | Workspaces > Global lists every Library skill with an exact enabled-Agent count; selection uses an explicit All agents or individual target picker |
| Project registration | Meets | Workspaces > Projects > Create project asks only for an existing directory, derives the default name, detects supported Agents, then previews one typed edit |
| Project Common/per-agent skills | Meets | Project > Common or Agent shows every Library skill, effective ownership, direct toggle, and Select skills atomic batch |
| Project name/path/agents | Meets | Project More separates Rename, Change project directory, and a five-Agent checklist; no pipe-delimited input remains |
| Presets CRUD and members | Meets | Typed create/edit/duplicate/rename/delete previews and force handling |
| Preset apply in workspace | Meets | Global and individual Agent rows expose the action directly; a selected Project Common/Agent target applies without asking for the target twice; Preset rows show applied scope usage |
| Library batch operations | Meets | Library More uses a structured scope picker and the app-owned atomic PreviewBatch/Batch contract |
| Workspace batch operations | Meets | Agent, Project Common, and Project Agent pages expose Select skills with search, multi-select, exact preview, and one atomic Batch |
| Effective state provenance | Meets | Workspace rows distinguish Direct, Preset, Project common, Global inherited, mixed ownership, and Conflict; inherited-only rows do not promise a false Disable |
| Status and explicit recovery | Meets | Structured issues, retry/sync preview, exact recovery review/resume |
| Keyboard/mouse/narrow operation | Meets generally | Shared geometry and parity tests exist; every new picker must retain the same bar |
| Marketplace, tags, archives | Excluded | Not part of aikit's approved local Git/path first-version scope |
| Custom tools/path overrides | Excluded | Aikit intentionally supports five fixed Agent adapters in this version |
| Backup and multi-machine sync | Excluded | Explicitly outside the approved single-machine scope |
| Activity logs and desktop updates | Excluded | Desktop-product functionality, not required for the CLI/TUI manager |

The excluded rows intentionally remain different from the current desktop
skills-manager product. The in-scope routes above are covered by
`TestSkillsManagerParityRoutesAreVisibleAndStructured`, project registration
and management tests, structured scope picker tests, and workspace projection
keyboard/mouse tests at wide and narrow terminal widths.

## Usability closeout

The final interaction audit also verifies the following terminal-specific
equivalents of skills-manager's visible workspace controls:

- the navigation rail prints the real `1`-`6` shortcuts instead of hiding them
  in Help;
- a focused action names itself in the footer (`Enter Create project`, for
  example) rather than using the generic `Enter Open` copy;
- the Agents collection reports all five supported adapters instead of calling
  an empty binding map `0 configured`;
- Project targets preserve the semantic order `Common`, then configured Agents;
- from a visible Agent or Project Common/Agent target, applying an existing
  Preset is three deliberate actions: **Apply preset**, choose the Preset,
  **Confirm**; the already-selected target is never requested again; and
- when no Preset exists, workspace pages offer **Create preset** instead of
  opening an empty picker.

These routes are exercised through both real keyboard messages and mouse hit
regions, including a 38-column terminal, in `usability_audit_test.go`.
