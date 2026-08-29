# README Top Preview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the existing README demo above the product rationale and remove the project-status section without allowing either layout decision to regress.

**Architecture:** Keep the generated GIF and all product copy unchanged. Update both localized README structures together, and extend the Bash documentation checker with explicit forbidden-heading and localized heading-order contracts backed by self-test fixtures.

**Tech Stack:** Markdown, Bash 3.2-compatible shell, existing documentation checker.

---

### Task 1: Capture the immutable GIF baseline and update good fixtures

**Files:**
- Modify: `scripts/check-docs-test.sh`
- Test: `scripts/check-docs-test.sh`

- [ ] Record `shasum -a 256 docs/assets/aikit-demo.gif` before editing any
      implementation file and retain the exact digest for final comparison.
- [ ] Update the good English fixture to order `## Preview`, `## Why aikit`,
      and `## Features`, and remove `## Project status`.
- [ ] Apply the equivalent localized structure to the Chinese fixture.
- [ ] Run `bash scripts/check-docs-test.sh` and verify RED at the initial good
      fixture because the old checker still requires the removed status heading.

### Task 2: Reject restored status headings

**Files:**
- Modify: `scripts/check-docs.sh`
- Modify: `scripts/check-docs-test.sh`
- Test: `scripts/check-docs-test.sh`

- [ ] Add English and Chinese failure fixtures that restore the removed status
      heading and expect `must not contain` diagnostics.
- [ ] Add a `forbid_literal` helper that reports a failure when a removed status
      heading occurs in its localized README.
- [ ] Add Preview to both required heading arrays and remove both status headings.
- [ ] Enforce both localized forbidden headings.
- [ ] Run `bash scripts/check-docs-test.sh` and verify PASS.

### Task 3: Enforce the localized heading order

**Files:**
- Modify: `scripts/check-docs.sh`
- Modify: `scripts/check-docs-test.sh`
- Test: `scripts/check-docs-test.sh`

- [ ] Add English and Chinese failure fixtures that move Preview below Features
      and expect localized heading-order diagnostics.
- [ ] Run `bash scripts/check-docs-test.sh` and verify RED because the current
      checker accepts the first misplaced Preview fixture.
- [ ] Add a Bash 3.2-compatible `require_heading_order` helper that resolves
      exact heading line numbers and requires Preview < Why < Features.
- [ ] Enforce the localized order in both READMEs.
- [ ] Run `bash scripts/check-docs-test.sh` and verify PASS.

### Task 4: Reorder both README openings

**Files:**
- Modify: `README.md`
- Modify: `README.zh-CN.md`

- [ ] Move the existing English Preview block immediately after the two-paragraph
      introduction and before `## Why aikit`.
- [ ] Delete the complete English `## Project status` section through the line
      before `## Features`.
- [ ] Apply the equivalent moves and deletion to Simplified Chinese.
- [ ] Confirm the GIF reference and both captions are byte-for-byte unchanged.
- [ ] Re-run `shasum -a 256 docs/assets/aikit-demo.gif` and compare it with the
      recorded baseline digest.
- [ ] Run `bash scripts/check-docs.sh` and verify PASS.

### Task 5: Verify and integrate

**Files:**
- Verify all changed and generated files on `docs/readme-tui-demo`.

- [ ] Run `bash -n scripts/check-docs.sh scripts/check-docs-test.sh`.
- [ ] Run `make check`.
- [ ] Run `git diff --check` and inspect README heading order and repository status.
- [ ] Export `GPG_TTY="$(tty)"`, refresh the agent with
      `gpg-connect-agent updatestartuptty /bye`, and run a disposable signed-data
      preflight with the configured signing key. If signing still fails, stop
      and ask the user to unlock or repair GPG; never disable commit signing.
- [ ] Commit all worktree changes on `docs/readme-tui-demo` with signed commits.
- [ ] Fast-forward merge `docs/readme-tui-demo` into the clean local `main` worktree.
- [ ] Re-run `make check` and the GIF SHA-256 comparison on local `main`, then
      confirm `main` is ahead of `origin/main` without pushing.
