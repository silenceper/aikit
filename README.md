**English** · [简体中文](README.zh-CN.md)

# aikit

[![CI](https://img.shields.io/github/actions/workflow/status/silenceper/aikit/ci.yml?branch=main&style=flat-square&label=CI&logo=githubactions&logoColor=white)](https://github.com/silenceper/aikit/actions/workflows/ci.yml)
[![Version](https://img.shields.io/github/v/release/silenceper/aikit?include_prereleases&sort=semver&style=flat-square&label=Version)](https://github.com/silenceper/aikit/releases)
[![Homebrew](https://img.shields.io/badge/Homebrew-silenceper%2Ftap-FBB040?style=flat-square&logo=homebrew&logoColor=black)](https://github.com/silenceper/homebrew-tap/blob/main/Formula/aikit.rb)
[![License](https://img.shields.io/github/license/silenceper/aikit?style=flat-square)](LICENSE)

**aikit is a local-first skills manager for AI coding agents.** It keeps one
authoritative ledger and one central skill library, then reconciles managed
links into the global and project workspaces used by Cursor, Claude Code,
Codex, GitHub Copilot, and Windsurf.

Use the full-screen, mouse-capable TUI for daily work or the deterministic CLI
for scripts and CI.

## Why aikit

Agent skills tend to become scattered copies across IDE-specific directories.
That makes basic questions surprisingly difficult:

- Which copy is authoritative?
- Where is a skill enabled?
- Did a project copy drift from the library?
- Can an update or cleanup finish safely after interruption?

aikit gives each concern one owner:

- `$AIKIT_HOME/config.yaml` is the durable ledger.
- `$AIKIT_HOME/library/skills/<id>` is the central content library.
- Agent directories contain managed links instead of independent copies.
- Global, project, and preset bindings refer to stable skill IDs.
- Pending operations and library journals preserve explicit recovery state.

## Project status

> [!IMPORTANT]
> aikit is alpha software. Its safety model is production-oriented, but CLI,
> TUI, configuration, and recovery metadata may still change before v1. Keep a
> recoverable backup of important configuration and review dry-run output
> before adopting existing directories.

This documentation describes the current `main` branch. Tagged alpha releases
are published through Homebrew and GitHub Releases and capture the exact
feature set at their tagged revision; `main` may contain newer changes.

The current release manages **Skills only**. Rules, MCP configuration, command
packs, a Web UI, export/import, cross-machine synchronization, and enterprise
support are intentionally out of scope.

macOS and Linux are the primary runtime targets. Release builds are configured
for Linux, macOS, and Windows on amd64 and arm64, and CI cross-compiles each
target. Cross-compilation is not native behavioral certification. In
particular, authenticated link recovery that requires anchored Unix directory
operations is unavailable on Windows and other unsupported platforms; those
operations fail closed instead of continuing unsafely.

See the [changelog](CHANGELOG.md) for release history and known evolution.

## Features

- **One central library** — discover, hash, add, inspect, update, compare, and
  remove skills without treating agent directories as independent sources.
- **Local and Git sources** — accept a local directory, Git URL, GitHub
  `owner/repo` shorthand, or an exact `skills.sh/<owner>/<repo>/<skill>` URL.
- **Global and project scopes** — enable skills globally, for project-common
  use, or for a specific Agent in a registered project.
- **Reusable presets** — maintain named skill sets and apply them to exact
  global or project scopes.
- **Interactive TUI** — keyboard and mouse navigation, responsive layouts,
  previews, exact confirmations, filtering, multi-select actions, and readable
  activity state.
- **Offline local inventory** — startup scans supported global roots and every
  registered project without fetching Git or mutating the ledger.
- **Updates and pinned refs** — cache branch update checks; keep tag and commit
  refs pinned; preserve the original Git transport.
- **Explicit migration and recovery** — preview import/adopt decisions and
  review pending recovery before new mutations proceed.
- **Fail-closed cleanup** — preserve unknown or replaced content when managed
  ownership cannot be proven.

## Preview

<!-- TUI screenshots will be added after the visual layout is frozen. -->

## Supported agents and platforms

| Agent ID | Global skill directory | Project skill directory |
|---|---|---|
| `cursor` | `~/.cursor/skills` | `<project>/.cursor/skills` |
| `claude-code` | `~/.claude/skills` | `<project>/.claude/skills` |
| `codex` | `~/.codex/skills` | `<project>/.codex/skills` |
| `copilot` | `~/.copilot/skills` | `<project>/.agents/skills` |
| `windsurf` | `~/.codeium/windsurf/skills` | `<project>/.windsurf/skills` |

| Platform | Release build | Current support boundary |
|---|---|---|
| macOS amd64/arm64 | Yes | Primary runtime target |
| Linux amd64/arm64 | Yes | Primary runtime target |
| Windows amd64/arm64 | Yes | Builds and baseline workflows; anchored link recovery fails closed |

## Installation

### Homebrew (recommended)

The formula is published in [`silenceper/tap`](https://github.com/silenceper/homebrew-tap).
Homebrew requires explicit trust for third-party taps, so trust only the aikit
formula before installing it:

```bash
brew tap silenceper/tap
brew trust --formula silenceper/tap/aikit
brew install silenceper/tap/aikit
aikit version
```

Upgrade or remove aikit with:

```bash
brew update
brew upgrade aikit

brew uninstall aikit
brew untrust --formula silenceper/tap/aikit
brew untap silenceper/tap
```

### Build from source

Go 1.25 or newer is required. Use a source build when developing aikit or
testing changes that have not reached a tagged release.

```bash
git clone https://github.com/silenceper/aikit.git
cd aikit
make build
./bin/aikit version
```

### Release archives

Download the archive for your operating system and architecture from
[GitHub Releases](https://github.com/silenceper/aikit/releases), then verify it
against `checksums.txt` from the same release before placing `aikit` on your
`PATH`.

Release archives are `.tar.gz` on Linux and macOS and `.zip` on Windows.

## Quick start

### 1. Add a skill

Add a local skill and enable it globally for Codex:

```bash
aikit add ./my-skill --agent codex
```

Add selected skills from a Git repository:

```bash
aikit add vercel-labs/agent-skills --skill code-review
aikit add https://skills.sh/vercel-labs/agent-skills/find-skills
aikit add https://gitlab.example.com/team/skills.git \
  --skill review --skill release
```

An exact `skills.sh` page suggests that page's skill. A repeatable `--skill`
always overrides the suggestion. GitHub `owner/repo` shorthand is resolved to
GitHub. aikit clones the underlying repository directly and does not execute
`npx` or depend on a marketplace API.

### 2. Register a project

```bash
aikit project add /path/to/project \
  --name my-project \
  --agent cursor \
  --agent codex
```

The TUI offers a path-first workflow that derives the name and detects Agent
directories automatically before showing an exact preview.

### 3. Enable a project skill

```bash
aikit enable vercel-labs/agent-skills/code-review \
  --project my-project \
  --agent codex
```

Omit `--agent` with `--project` to target the project's common skill scope.

### 4. Create and apply a preset

```bash
aikit preset create essentials \
  --skill vercel-labs/agent-skills/code-review
aikit enable --preset essentials --agent cursor
```

### 5. Inspect and reconcile

```bash
aikit status --offline
aikit sync --dry-run
aikit sync
```

## TUI

Run `aikit` without arguments from a terminal. Non-TTY environments must use an
explicit CLI command and never fall into an interactive screen.

The navigation rail provides these daily destinations:

- **Overview** — attention queue, update summary, and common entry points.
- **Library** — managed skills, details, source addition, refs, updates, and
  multi-select actions.
- **Presets** — reusable sets, membership, duplication, rename, delete, and
  exact-scope application.
- **Global / Agents / Projects** — direct access to workspace bindings and
  project management.
- **Configuration** — resolved config, library, and cache paths plus read-only
  validation and reload.

Health details, local import review, and explicit recovery are available from
the command palette when relevant.

Common controls:

| Key | Action |
|---|---|
| Mouse click / wheel | Select, activate, or scroll the same visible controls |
| `j` / `k`, arrows | Move within the focused pane |
| `Tab` / `Shift+Tab` | Move focus between navigation, list, details, and actions |
| `Enter` | Open or activate the focused item |
| `Space` | Select a row or toggle a binding where available |
| `/` | Filter the current collection |
| `Ctrl+P` or `:` | Open the command palette |
| `Ctrl+K` | Open Configuration directly |
| `Esc` | Go back or cancel; it does not quit from the main screen |
| `Ctrl+Q` | Quit normally |
| `Ctrl+C` | Emergency exit, including during an operation |
| `?` | Open contextual help |

Startup first renders the local ledger, then incrementally inventories all
supported global Agent roots and registered projects. Startup inventory is
offline and read-only: it does not fetch Git, import content, adopt paths,
rewrite configuration, or mutate the central library.

Mutating workflows build a typed preview and require explicit confirmation.
For a remote **Add source**, the first confirmation permits only temporary Git
discovery; a second screen selects exact candidates before anything is added.

## CLI

The CLI covers deterministic automation and supports global `--json` output.

```text
aikit add <source-or-path> [--skill ...] [--agent ...] [--project ...]
aikit list [--agent ...] [--project ...] [--preset ...] [--offline]
aikit remove <id> [--force] [--yes]
aikit enable [id] | --preset <name> --agent/--project
aikit disable [id] | --preset <name> --agent/--project
aikit preset create|add|remove|list
aikit project add|edit|remove|list
aikit sync [--agent ...] [--project ...] [--dry-run]
aikit status [--offline] [--refresh]
aikit update [id] [--skill ...] [--check] [--yes]
             [--ref branch:...|tag:...|commit:...]
aikit scan [--agent ...] [--project ...] [--skill ...] [--all] [--adopt]
aikit migrate [--project ...] [--dry-run] [--adopt]
```

Use `aikit <command> --help` for the authoritative flags.

The TUI currently exposes some advanced workflows that do not have a dedicated
CLI command, including Configuration validation/reload, atomic multi-action
Library batches, preset rename/duplicate, skill comparison, and explicit
recovery review/resume. Presets can still be created, edited, listed, removed,
and applied from the CLI.

### Updates and refs

Remote skills record their canonical source, repository-relative source path,
structured ref, and full resolved object ID. Branch refs participate in update
checks; tag and commit refs remain pinned.

```bash
aikit update --check
aikit update --yes
aikit update <id> --ref tag:v2.0.0 --force --yes
aikit status --offline
```

In a non-TTY environment, an update without `--yes` is check-only.

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Operation completed without a reported issue |
| `1` | Error or partial result, including drift, conflict, or pending recovery |
| `2` | Updates are available but nothing was changed |

## Configuration and data

`AIKIT_HOME` defaults to `~/.aikit` and must resolve to an absolute local path.

| Path | Purpose |
|---|---|
| `$AIKIT_HOME/config.yaml` | Durable global ledger |
| `$AIKIT_HOME/config.lock` | Cross-process mutation lock |
| `$AIKIT_HOME/library/skills/<id>` | Central skill content |
| `$AIKIT_HOME/cache/` | Git mirrors, temporary source data, and update cache |

Set a separate home for testing or isolated profiles:

```bash
AIKIT_HOME=/absolute/path/to/aikit-home aikit status --offline
```

Do not hand-edit the ledger while aikit is running. Keep a backup before manual
repair, preserve pending-operation fields, and use TUI recovery review rather
than deleting journal or quarantine files.

## Safety and recovery

aikit's filesystem model is designed to fail closed:

- mutations are serialized by `$AIKIT_HOME/config.lock`;
- configuration checkpoints use atomic replacement;
- Library changes use staged, journaled, ledger-directed mutations;
- normal reconciliation does not overwrite a real directory or unknown file;
- destructive cleanup authenticates expected managed objects and preserves
  replaced or unrecognized content;
- source discovery and content reads use containment and no-follow checks;
- authorization headers, embedded credentials, and sensitive URLs are redacted
  from errors;
- preview, dry-run, status-offline, and startup inventory paths are read-only;
- pending recovery blocks unrelated mutations until it is reviewed explicitly.

No local filesystem tool can protect against every privileged or hostile
same-user process. Keep operating-system backups for important work and inspect
unexpected conflicts instead of deleting `.aikit-*` artifacts manually.

## Troubleshooting

### `aikit` says a command is required

stdin is not a TTY, so aikit correctly refused to open the full-screen UI. Use
an explicit command such as `aikit list --offline` or `aikit status --offline`.

### A path is occupied or cleanup is blocked

Run:

```bash
aikit status --offline
aikit sync --dry-run
```

aikit preserves unknown content instead of overwriting it. Inspect the reported
path and move or reconcile it manually only after verifying ownership.

### A pending recovery blocks changes

Open the TUI and choose **Review recovery** from the command palette or
attention queue. Review the exact operation IDs and paths before resuming.
Do not delete pending ledger entries or `.aikit-*` artifacts by hand.

### Remote update checks fail

Use `aikit status --offline` to inspect local state without network access.
Check that the same Git URL works with your normal Git credential helper or SSH
agent. aikit rejects HTTP(S) sources with embedded userinfo; do not put tokens
in source URLs.

### aikit uses an unexpected configuration

Check `AIKIT_HOME`, then press `Ctrl+K` in the TUI to view the resolved config,
Library, and cache paths.

## Contributing and support

- Read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request.
- Use [SUPPORT.md](SUPPORT.md) to choose the correct support channel.
- Report vulnerabilities privately according to [SECURITY.md](SECURITY.md).
- All community participation follows the [Code of Conduct](CODE_OF_CONDUCT.md).
- User-visible changes are tracked in [CHANGELOG.md](CHANGELOG.md).

Development verification:

```bash
make check
make test-e2e
```

The end-to-end test uses only temporary local directories and a local skill; it
does not require network access or alter real agent directories.

## License

aikit is licensed under the [Apache License 2.0](LICENSE).
