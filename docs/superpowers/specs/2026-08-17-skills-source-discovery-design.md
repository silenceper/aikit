# Skills Source Discovery Design

Date: 2026-08-17
Status: Approved by user

## Goal

Make Library > Add source accept local directories, GitHub/Git transports, and
individual `skills.sh` skill-page URLs. Remote repositories are discovered only
after explicit network confirmation, then shown as a selectable checklist
before any Library or config mutation.

## Chosen approach

Resolve `skills.sh` URLs locally instead of depending on the skills.sh API or
the `npx skills` CLI. A URL shaped as
`https://skills.sh/<owner>/<repo>/<skill>` resolves to the GitHub repository
`https://github.com/<owner>/<repo>.git` with `<skill>` preselected. A two-part
repository page may resolve to the repository without a preselection. Other
skills.sh routes, query strings, fragments, credentials, or ambiguous paths
fail closed.

GitHub shorthand (`owner/repo`) similarly resolves to an actual HTTPS clone
URL. Existing full HTTPS/SSH/SCP Git transports and local paths retain their
current meaning.

## Remote discovery flow

1. The initial `PreviewAdd` remains offline and read-only. It resolves the
   source and reports that explicit network access is required.
2. After the user confirms network access, a second typed preview clones into a
   private temporary directory, checks out the requested/default ref, discovers
   every `SKILL.md`, returns candidates plus the resolved commit, then removes
   the temporary checkout. It does not touch config, Library, pending journals,
   or the persistent repository cache.
3. The TUI displays the candidates as a multi-select checklist. A skills.sh
   skill-page candidate starts selected; a repository source requires the user
   to choose at least one candidate.
4. The final confirmation carries the selected candidate hashes and resolved
   Git object. `Add` revalidates both after preparing the real mutation and
   before checkpointing config. A changed remote fails closed and asks the user
   to discover again.

## Interface changes

- `AddPreviewRequest` gains an explicit `AllowNetwork` flag.
- `AddPreview` returns `ResolvedSource`, `SuggestedSelections`, `Ref`, and
  `Resolved` in addition to candidates and warnings.
- `AddRequest` carries the expected resolved object and selected candidate
  hashes.
- The library adapter uses one shared source resolver for preview and mutation.

## TUI behavior

- `Library > Add source` accepts a local path, Git URL, `owner/repo`, or exact
  skills.sh skill URL.
- Network confirmation never performs the add itself; it only discovers.
- Candidate selection uses the existing `ModeAddSelect` keyboard/mouse
  checklist and a separate final confirmation.
- Cancel at either confirmation or selection performs no Library/config write.
- Errors name whether parsing, network discovery, candidate matching, or remote
  revalidation failed.

## CLI behavior

Existing Git and local commands remain compatible. Exact skills.sh skill URLs
automatically supply their page skill when `--skill` is absent; an explicit
`--skill` remains authoritative. GitHub shorthand becomes a real HTTPS clone
source instead of being passed literally to Git.

## Safety and tests

- Reject unsafe skills.sh URL variants and encoded path separators.
- Assert remote discovery leaves config, Library, cache, and pending state
  byte-for-byte unchanged and cleans its temporary checkout on success/error.
- Test two-stage TUI keyboard/mouse flows, multi-selection, default skills.sh
  selection, cancellation, and busy gating.
- Test remote object/hash changes between discovery and final add are rejected
  before config checkpoint or Library commit.

