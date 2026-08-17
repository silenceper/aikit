# TUI Skills Manager Parity Audit

Date: 2026-08-17
Status: Implementation baseline

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
| All Agents global workspace | Missing | `Global` is a non-operable detail row; implement aggregate skill state and explicit agent targets |
| Project registration | Missing usability | Current TUI requires `name|path|agents`; replace with directory-only preview |
| Project Common/per-agent skills | Partial | Bindings work after registration; management and provenance need clearer project-local entry |
| Project name/path/agents | Missing usability | Current TUI requires encoded edit fields; replace with independent inputs and checklist |
| Presets CRUD and members | Meets | Typed create/edit/duplicate/rename/delete previews and force handling |
| Preset apply in workspace | Partial | Backend exists; target is an encoded string and workspace pages lack direct entry |
| Library batch operations | Partial | Atomic mutations exist; scope preview is composed in the TUI and selected through encoded text |
| Workspace batch operations | Missing | Add a searchable Library multi-picker and app-owned atomic preview for current scope |
| Effective state provenance | Missing | Rows do not distinguish Direct, Preset, Project Common, or Global inherited ownership |
| Status and explicit recovery | Meets | Structured issues, retry/sync preview, exact recovery review/resume |
| Keyboard/mouse/narrow operation | Meets generally | Shared geometry and parity tests exist; every new picker must retain the same bar |
| Marketplace, tags, archives | Excluded | Not part of aikit's approved local Git/path first-version scope |
| Custom tools/path overrides | Excluded | Aikit intentionally supports five fixed Agent adapters in this version |
| Backup and multi-machine sync | Excluded | Explicitly outside the approved single-machine scope |
| Activity logs and desktop updates | Excluded | Desktop-product functionality, not required for the CLI/TUI manager |

Release completion requires every in-scope `Partial` or `Missing` row above to
become `Meets`, with an exact TUI route and an automated keyboard/mouse test.
