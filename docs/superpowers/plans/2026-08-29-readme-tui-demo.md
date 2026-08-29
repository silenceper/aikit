# README TUI Demo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Record and embed a polished, reproducible aikit first-run import and Skill-management demo.

**Architecture:** A small seed script creates fictional Skills under an explicit isolated home. A versioned VHS tape builds and runs the real aikit binary against a temporary `HOME` and `AIKIT_HOME`, performs the workflow with keyboard input, and writes a GIF consumed by both READMEs. The render wrapper normalizes the GIF to a real 12 fps and validates its dimensions, duration, frame count, and size.

**Tech Stack:** Bash, Go 1.25, Bubble Tea TUI, Charmbracelet VHS, GIF, Markdown.

---

### Task 1: Add isolated demo data

**Files:**
- Create: `docs/demo/seed.sh`

- [ ] Support `prepare`, `verify`, and `clean` operations against the fixed
      `/tmp/aikit-readme-demo` root.
- [ ] Reject empty, relative, `/`, real-home, symlink, and unmarked existing
      paths; create and validate an exact ownership marker.
- [ ] Create Codex, Claude Code, and Cursor global Skill directories below that home.
- [ ] Write three valid fictional `SKILL.md` files with no private data.
- [ ] Run the script against a temporary directory and validate all three files.
- [ ] Run `bash -n docs/demo/seed.sh`.

### Task 2: Add the reproducible VHS tape

**Files:**
- Create: `docs/demo/aikit.tape`

- [ ] Configure a readable wide terminal, dark theme, frame rate, padding, and GIF output.
- [ ] In hidden setup, prepare the fixed marker-owned root; set `HOME`,
      `AIKIT_HOME`, `GOCACHE`, `GOMODCACHE`, `GOPATH`, and `XDG_CACHE_HOME`
      beneath it; then build current aikit. Preserve the real `HOME` first so
      all safety checks compare against the original value.
- [ ] Seed the demo home and start the real TUI.
- [ ] Wait for completed startup inventory, select all local import candidates,
      and confirm the import preview.
- [ ] Wait for the post-import Library refresh, filter and inspect a Skill,
      then enable `release-checklist` for **Global / codex**.
- [ ] Use screen-content waits for asynchronous transitions and sleeps only for
      presentation pacing.
- [ ] After quitting normally, run the final-state assertions in hidden
      teardown, then clean only the generated temporary directory. Keep a trap
      as cleanup fallback when recording or assertions fail.

### Task 3: Render and visually inspect the GIF

**Files:**
- Create: `docs/assets/aikit-demo.gif`
- Create: `docs/demo/render.sh`

- [ ] Before rendering, require Go, ffmpeg, a non-login shell, Menlo, and
      VHS >= 0.9.0; if VHS is missing, fail clearly with an installation hint
      instead of silently changing the user's global tool environment.
- [ ] Record the Go and VHS versions used for generation.
- [ ] Render with `docs/demo/render.sh` after its fail-fast check passes.
- [ ] Confirm the GIF is non-empty, below 10 MiB, 1100 x 660, and approximately
      20 to 30 seconds at a probed 12 fps.
- [ ] Extract first, middle, and final frames with ffmpeg.
- [ ] Inspect the extracted frames for readability, clipping, secrets, stale UI,
      and coherent story progression.
- [ ] Adjust tape timing or dimensions and rerender until all checks pass.
- [ ] Run the tape a second time to verify the workflow remains reproducible;
      both runs must assert three Library Skills, all three source directories,
      exactly one Codex binding to `release-checklist`, the managed link target,
      and zero pending operations before teardown cleanup.

### Task 4: Embed the demo in both READMEs

**Files:**
- Modify: `README.md`
- Modify: `README.zh-CN.md`

- [ ] Replace the Preview placeholder with the GIF and a concise English caption.
- [ ] Mirror the same asset with localized Chinese copy.
- [ ] Keep the surrounding README structure unchanged.

### Task 5: Enforce and verify the documentation contract

**Files:**
- Modify: `scripts/check-docs.sh`
- Modify: `scripts/check-docs-test.sh`

- [ ] Require both READMEs to reference `docs/assets/aikit-demo.gif`.
- [ ] Require the GIF to be non-empty and smaller than 10 MiB.
- [ ] Add self-tests proving that a missing README reference, missing GIF,
      empty GIF, and oversized GIF each fail clearly.
- [ ] Run `make check-docs`.
- [ ] Run `git diff --check` and inspect the final diff and repository status.
