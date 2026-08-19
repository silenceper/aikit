# Contributing to aikit

Thank you for helping improve aikit. Contributions of code, tests,
documentation, bug reports, and design feedback are welcome.

By participating, you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
Security vulnerabilities must be reported privately as described in
[SECURITY.md](SECURITY.md), not through a public issue.

## Before you start

- Search existing issues and pull requests before opening a duplicate.
- For a substantial feature or behavior change, open an issue first so the
  problem, user workflow, and safety constraints can be agreed before code is
  written.
- Keep each pull request focused. Unrelated refactors make safety-sensitive
  filesystem changes harder to review.

## Development setup

Requirements:

- Go 1.25 or newer
- Git
- Bash for repository scripts and end-to-end tests

```bash
git clone https://github.com/silenceper/aikit.git
cd aikit
go mod download
make check
```

`make check` runs the documentation contract, formatting and module checks,
native and cross-platform builds, the race-enabled test suite, and `go vet`.

## Repository layout

- `cmd/` — Cobra CLI and frontend wiring
- `internal/app/` — application-level orchestration and mutation boundaries
- `internal/library/` — discovery, Git sources, staged mutations, and recovery
- `internal/link/` — link planning, authenticated cleanup, and reconciliation
- `internal/migrate/` — offline inventory, import, adopt, and legacy migration
- `internal/tui/` — Bubble Tea terminal interface
- `pkg/config/` — public ledger model, validation, locking, and checkpoints
- `scripts/` — offline repository checks and end-to-end tests

## Change workflow

1. Add or update a focused test that demonstrates the desired behavior.
2. Run it and confirm that it fails for the expected reason.
3. Implement the smallest complete change.
4. Run focused tests, then `make check` and `make test-e2e`.
5. Update both `README.md` and `README.zh-CN.md` when public behavior changes.
6. Explain compatibility, filesystem, credential, and recovery impact in the
   pull request.

Useful focused commands:

```bash
go test ./internal/app -run TestName -count=1
go test -race ./internal/tui -count=1
go vet ./...
make check-docs
make test-e2e
```

The end-to-end test uses temporary directories and a local skill. It must not
alter the contributor's real agent directories or require network access.

## Safety expectations

aikit manages user-authored files and links, so safety regressions are release
blockers. Changes that mutate the filesystem must preserve these properties:

- never overwrite an unknown file or real directory;
- keep all operations inside their authorized library or workspace root;
- fail closed when ownership or object identity cannot be proven;
- make interrupted mutations recoverable through durable state;
- do not leak source credentials or authorization headers in errors;
- keep preview, dry-run, and offline startup paths read-only.

Add platform-specific tests when behavior depends on symlinks, file identity,
atomic replacement, permissions, or process locking. Cross-compilation proves
that code builds; it does not replace native behavioral testing.

## Pull requests

Pull requests should include:

- a concise problem statement and solution summary;
- linked issues where applicable;
- RED/GREEN evidence for behavior changes;
- the exact verification commands that passed;
- screenshots only when they materially explain a TUI change;
- documentation updates for user-visible behavior;
- a note about data, security, compatibility, and recovery impact.

Use clear, imperative commit messages. Conventional prefixes such as `feat:`,
`fix:`, `docs:`, `test:`, and `chore:` are encouraged but not required.

Maintainers may ask for a change to be split when independent concerns make it
difficult to review safely.

## License

By submitting a contribution, you agree that it may be distributed under the
project's [Apache License 2.0](LICENSE).
