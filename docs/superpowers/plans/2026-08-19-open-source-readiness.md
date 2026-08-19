# Open Source Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make aikit's public repository understandable, supportable, and release-ready through truthful bilingual documentation, standard governance files, repository templates, and locally verifiable CI checks.

**Architecture:** Keep `README.md` as the English default and mirror its user-facing structure in `README.zh-CN.md`. Put stable process and policy details in focused root documents, enforce the public documentation contract with a small offline shell check, and extend GitHub Actions without adding third-party services or secrets. Existing Homebrew publication configuration remains untouched and unadvertised until it is separately verified; screenshot assets are explicitly out of scope.

**Tech Stack:** Markdown, POSIX-compatible shell, GitHub Actions, Go 1.25

---

### Task 1: Add a failing public-documentation contract check

**Files:**
- Create: `scripts/check-docs.sh`
- Create: `scripts/check-docs-test.sh`
- Modify: `Makefile`

- [ ] **Step 1: Write the failing check**

Create an offline script that requires the English and Chinese READMEs, governance files, GitHub issue/PR templates, language switch links, essential README headings, no active Homebrew installation command, and valid relative Markdown links.

- [ ] **Step 2: Run it to verify RED**

Run: `bash scripts/check-docs.sh`
Expected: FAIL because `README.zh-CN.md` and governance/template files do not exist.

- [ ] **Step 3: Add negative self-tests for the checker**

Build temporary fixtures from known-good minimal documents and prove the checker rejects, independently: a broken relative link, an active Brew installation command, a missing reciprocal language link, a missing English heading, and a missing Chinese heading. It must also accept the complete fixture.

- [ ] **Step 4: Add `make check-docs`**

Expose the same deterministic check for contributors and CI. The target must run `bash scripts/check-docs-test.sh` first and then `bash scripts/check-docs.sh`, so a broken checker cannot make the repository gate pass silently.

### Task 2: Add open-source governance and repository templates

**Files:**
- Create: `CONTRIBUTING.md`
- Create: `SECURITY.md`
- Create: `CODE_OF_CONDUCT.md`
- Create: `SUPPORT.md`
- Create: `CHANGELOG.md`
- Create: `.github/ISSUE_TEMPLATE/bug_report.yml`
- Create: `.github/ISSUE_TEMPLATE/feature_request.yml`
- Create: `.github/ISSUE_TEMPLATE/documentation.yml`
- Create: `.github/ISSUE_TEMPLATE/config.yml`
- Create: `.github/pull_request_template.md`

- [ ] **Step 1: Verify the private security-reporting channel**

Confirm that GitHub private vulnerability reporting is available at `https://github.com/silenceper/aikit/security/advisories/new`. If it is unavailable, stop rather than inventing an email address or directing vulnerabilities to a public issue.

- [ ] **Step 2: Add focused governance documents**

Document supported versions, private security reporting, contributor setup and verification, support boundaries, Contributor Covenant expectations, and the alpha release history without inventing service guarantees or contact details.

- [ ] **Step 3: Add structured GitHub templates**

Collect reproducible environment data, distinguish feature/documentation requests, point security reports away from public issues, and require PR authors to report tests plus data/security/compatibility impact.

- [ ] **Step 4: Re-run the contract check**

Run: `bash scripts/check-docs.sh`
Expected: still FAIL until both READMEs satisfy the public contract.

### Task 3: Rewrite the English README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Replace the project homepage structure**

Add language links, truthful badges, alpha status, core value, supported agents/platforms, release/source installation, a short quick start, TUI and CLI capability boundaries, configuration/data layout, safety/recovery, troubleshooting, development, governance links, and license. Platform wording must distinguish release build targets from native runtime-tested support; Windows documentation must disclose that some authenticated recovery operations fail closed when the required anchored filesystem primitive is unavailable.

- [ ] **Step 2: Keep screenshots as a non-rendering placeholder**

Reserve the section with an HTML comment so the page has no broken asset or fake screenshot.

- [ ] **Step 3: Verify every command against Cobra help**

Run: `go run . --help` plus focused subcommand help commands.
Expected: documented commands and flags exist; TUI-only capabilities are not represented as CLI commands.

### Task 4: Add the complete Simplified Chinese README

**Files:**
- Create: `README.zh-CN.md`

- [ ] **Step 1: Mirror the English information architecture**

Translate the complete user-facing content while keeping commands, paths, IDs, agent names, exit codes, and safety terminology precise.

- [ ] **Step 2: Keep language navigation reciprocal**

Both READMEs must link to each other at the top, and their key public sections must remain structurally aligned.

- [ ] **Step 3: Run the documentation check to verify GREEN**

Run: `bash scripts/check-docs.sh`
Expected: PASS.

### Task 5: Harden CI with locally reproducible gates

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `Makefile`

- [ ] **Step 1: Add locally reproducible Make targets**

Add `check-docs`, `check-format`, `check-mod`, `check-build`, `check-test`, `check-vet`, `check-cross`, and aggregate `check` targets. `check-docs` runs both the checker self-tests and the repository check. `check-mod` uses non-mutating `go mod tidy -diff`. `check-test` runs `go test -race ./...`. `check-vet` runs `go vet ./...`. `check-cross` explicitly runs `CGO_ENABLED=0 go build ./...` for `linux/darwin/windows × amd64/arm64`. Aggregate `check` depends on every one of these gates.

- [ ] **Step 2: Add documentation and formatting gates**

Run `make check`, the same complete gate used by contributors, including documentation self-tests, formatting, module tidiness, native build, race tests, vet, and cross-builds.

- [ ] **Step 3: Add cross-platform compile gates**

Compile all packages with `CGO_ENABLED=0` for the explicit `linux/darwin/windows × amd64/arm64` matrix. These are compile gates, not claims of native behavioral coverage. Do not change existing Homebrew publication, add external quality services, or add new secrets.

- [ ] **Step 4: Validate workflow syntax and commands locally**

Inspect YAML with an available local parser and execute every shell command that is platform-independent.

### Task 6: Complete release-quality verification

**Files:**
- Verify all files above

- [ ] **Step 1: Run focused documentation checks**

Run: `make check-docs`
Expected: PASS.

- [ ] **Step 2: Run repository verification**

Run: `make check`, `go test ./... -count=1`, `go test -race ./... -count=1`, `go vet ./...`, and `make test-e2e`. The explicit six-target cross-build matrix runs inside `make check`.
Expected: all PASS; sandbox-only Go cache warnings are acceptable only with exit code 0.

- [ ] **Step 3: Check patch hygiene**

Run: `git diff --check`, `gofmt -l` on Go sources, and `git status --short`.
Expected: no whitespace errors, no Go formatting drift, and only intended files changed.

- [ ] **Step 4: Review public claims**

Compare README promises with command help, current tests, GoReleaser configuration, and platform-specific code. Remove any unsupported claim before completion.
