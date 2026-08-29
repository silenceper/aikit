# README TUI Demo Design

## Goal

Add a polished, reproducible terminal demo to both READMEs. The recording must
show real aikit behavior without reading or mutating the recorder's existing
AIKIT configuration or Agent directories.

## User story

The demo tells one short workflow:

1. Start aikit with three unmanaged local Skills already present in isolated
   Codex, Claude Code, and Cursor directories.
2. Open the **Local skills** panel and select all three candidates.
3. Review and confirm the exact import preview.
4. Open **Library**, filter the imported Skills, and inspect one Skill.
5. Select `release-checklist` and enable it for the isolated
   **Global / codex** workspace. This target is intentionally different from
   the Skill's original Claude Code directory, so the binding is conflict-free.

The result should make first-time import, safe confirmation, discovery, and
ongoing management understandable in roughly 20 to 30 seconds.

## Recording approach

Use Charmbracelet VHS because its `.tape` source makes the recording
repeatable and reviewable. The README embeds the generated GIF directly; it
does not depend on an external recording host or an unsupported inline player.

The tape runs the current source tree, not mocked output. Hidden setup steps:

- preserve the recorder's real `HOME` for explicit safety comparisons;
- create `/tmp/aikit-readme-demo`, guarded by an exact ownership marker;
- point `HOME`, `AIKIT_HOME`, `GOCACHE`, `GOMODCACHE`, `GOPATH`, and
  `XDG_CACHE_HOME` into that root before building or running anything;
- build the current `aikit` binary inside the isolated root;
- seed three small Skills under isolated global Agent directories;
- add the temporary binary to `PATH`.

The fixed root keeps visible source paths stable between recordings. Setup and
cleanup reject an empty path, `/`, the recorder's real home, relative paths,
symlinks, and unexpected existing directories. After the TUI exits, hidden
teardown first verifies the final state and then cleans only the marker-owned
root; a trap provides cleanup fallback if recording or assertions fail.

## Files

- `docs/demo/aikit.tape` — deterministic keyboard sequence and visual settings.
- `docs/demo/render.sh` — fail-fast dependency/version check, rendering entrypoint,
  deterministic 12 fps normalization, and artifact validation.
- `docs/demo/seed.sh` — prepares, verifies, and cleans the marker-owned demo
  root beneath an explicit absolute path.
- `docs/assets/aikit-demo.gif` — generated README asset.
- `README.md` and `README.zh-CN.md` — embed the same asset with localized copy.
- `scripts/check-docs.sh` and its test — keep the preview asset in the public
  documentation contract.

## Demo data

Use clearly fictional, non-sensitive Skills with distinct names and useful
descriptions:

- `code-review` in the isolated Codex global directory;
- `release-checklist` in the isolated Claude Code global directory;
- `api-design` in the isolated Cursor global directory.

Each Skill contains only a small valid `SKILL.md`. No private paths, usernames,
credentials, network calls, or user repository contents appear in the GIF.

## Visual quality

- Dark, high-contrast theme with a restrained terminal frame.
- Fixed 1100 x 660 output, Menlo 18 px text, and a verified 12 fps artifact.
- Wide layout that shows navigation, list, and detail panes together and stays
  readable at GitHub's rendered README width.
- Deliberate pauses around import preview, success state, Library detail, and
  scope confirmation.
- Target a readable GIF below 10 MiB; reduce frame rate or dimensions before
  sacrificing text clarity.

## Verification

- Use screen-content waits for completed startup inventory, migration preview,
  post-import Library refresh, Skill detail loading, binding preview, and final
  mutation success. Fixed sleeps are only for intentional presentation pauses.
- Run the tape from a clean marker-owned environment twice.
- Confirm the second render is operationally successful and visually stable.
- Inspect the GIF's first frame, representative middle frames, and final frame.
- Confirm no real home paths or secrets are visible.
- After the TUI exits, assert three Library Skills, all three original example
  directories, exactly one Codex binding to `release-checklist`, the expected
  managed link, and zero pending operations.
- Record the Go and VHS versions used for generation; fail early when required
  programs, VHS >= 0.9.0, or the Menlo font are unavailable.
- Require a non-empty GIF below 10 MiB and require both READMEs to reference
  the same asset.
- Run `make check-docs` and `git diff --check`.
