# aikit

`aikit` is a local-first skills manager for AI coding agents. It keeps one
global ledger and one central skill library, then reconciles symlinks into the
global and project workspaces used by Cursor, Claude Code, Codex, GitHub
Copilot, and Windsurf.

The first release manages **Skills only**. Rules, MCP, Commands, Web UI,
export/import, and cross-machine synchronization are intentionally out of
scope.

## Why

Agent skills are normally scattered across several IDE-specific directories.
That makes it difficult to answer basic questions: which copy is authoritative,
where a skill is enabled, whether two copies drifted, and how to update all
consumers safely.

`aikit` gives those concerns explicit owners:

- `$AIKIT_HOME/config.yaml` is the single ledger.
- `$AIKIT_HOME/library/skills/<id>` is the central content library.
- Agent directories contain managed symlinks, not independent copies.
- Presets and project bindings store skill IDs and are reconciled from the
  ledger.
- Pending cleanup/adopt operations and library journals make interrupted
  mutations recoverable.

When `AIKIT_HOME` is unset it defaults to `~/.aikit`.

## Supported agents

| Agent | Global skills | Project skills |
|---|---|---|
| `cursor` | `~/.cursor/skills` | `<project>/.cursor/skills` |
| `claude-code` | `~/.claude/skills` | `<project>/.claude/skills` |
| `codex` | `~/.codex/skills` | `<project>/.codex/skills` |
| `copilot` | `~/.copilot/skills` | `<project>/.agents/skills` |
| `windsurf` | `~/.codeium/windsurf/skills` | `<project>/.windsurf/skills` |

## Installation

```bash
brew tap silenceper/tap
brew install aikit
```

Or build from source:

```bash
make build
./bin/aikit version
```

## Quick start

Add a local skill to the library and enable it globally for Codex:

```bash
aikit add ./my-skill --agent codex
```

Add one or more skills from a Git repository:

```bash
aikit add vercel-labs/agent-skills --skill code-review
aikit add https://gitlab.example.com/team/skills.git \
  --skill review --skill release
```

Register a project, then enable a skill for one project agent:

```bash
aikit project add . --name my-project --agent cursor --agent codex
aikit enable vercel-labs/agent-skills/code-review \
  --project my-project --agent codex
```

Create and apply a preset:

```bash
aikit preset create essentials \
  --skill vercel-labs/agent-skills/code-review
aikit enable --preset essentials --agent cursor
```

Inspect and reconcile:

```bash
aikit status
aikit sync --dry-run
aikit sync
```

Running `aikit` without arguments opens the full-screen TUI when stdin is a
TTY. On every launch it first renders the local ledger, then scans all
supported global Agent roots and every registered project incrementally. This
startup inventory is offline and read-only: it does not fetch Git sources,
rewrite configuration, import skills, or adopt existing directories.

The TUI is English-first and supports both keyboard and mouse input. Use `1`–`6`
to switch sections, `j`/`k` or the arrow keys to move, `Tab` to move between the
list, details, and actions, `Enter` to activate, `/` to filter, `?` for help,
and the mouse wheel or clickable rows and buttons for the same operations.
Mutating actions show a preview and explicit confirmation before writing.

Missing required arguments never open a TUI in CI or another non-TTY
environment.

## TUI workspace

The six top-level sections keep the first screen intentionally compact:

- **Overview** summarizes library size, unmanaged skills, drift, updates, and
  pending recovery.
- **Library** shows centrally managed skills, details, add, remove, and update
  entry points.
- **Workspaces** groups global Agents and registered projects, including common
  and per-Agent bindings.
- **Presets** manages reusable skill sets.
- **Migration** presents exact local origins and the planned Import, Adopt,
  Link-existing, or Ignore action before confirmation.
- **Status** shows reconciliation issues and previews sync work.

Press `Ctrl+K` to open Configuration. The global YAML ledger is
`$AIKIT_HOME/config.yaml`; when `AIKIT_HOME` is unset the path is
`~/.aikit/config.yaml`. The configuration view also reports the resolved path,
so changing `AIKIT_HOME` is visible before any operation.

## Commands

```text
aikit add <source-or-path> [--skill ...] [--agent ...] [--project ...]
aikit list [--agent ...] [--project ...] [--preset ...]
aikit remove <id> [--force]
aikit enable <id> | --preset <name> --agent/--project
aikit disable <id> | --preset <name> --agent/--project
aikit preset create|add|remove|list
aikit project add|edit|remove|list
aikit sync [--agent ...] [--project ...] [--dry-run]
aikit status [--offline] [--refresh]
aikit update [id] [--check] [--yes] [--ref branch:...|tag:...|commit:...]
aikit scan [--agent ...] [--project ...] [--adopt]
aikit migrate [--project ...] [--dry-run] [--adopt]
```

Use `aikit <command> --help` for the complete flags.

## Updates and refs

Remote skills store a canonical source, the skill's repository-relative
`source_path`, a structured ref, and the full resolved object ID. Branch refs
participate in update checks; tag and commit refs are pinned.

```bash
aikit update --check
aikit update --yes
aikit update <id> --ref tag:v2.0.0 --yes
aikit status --offline
```

In a non-TTY environment, an update without `--yes` is check-only. Exit code
`2` means updates are available but nothing was changed. Exit code `1` means a
partial result, drift, conflict, pending recovery, or another error.

## Scan, adopt, and migration

`scan` discovers existing skills in supported agent directories and imports
their content without replacing those directories. `scan --adopt` is the
explicit authorization to replace a discovered directory with a managed
symlink; interrupted adoption is recorded and recoverable.

The previous `catalog.yaml` and project `.aikit.yaml` formats are not used as
the live ledger. Migrate them explicitly:

```bash
aikit migrate --dry-run
aikit migrate
aikit migrate --project /path/to/project --adopt
```

Migration reads old files but does not delete them.

## Safety model

- All mutations are serialized by `$AIKIT_HOME/config.lock`.
- The ledger is checkpointed atomically.
- Library changes use staged, journaled mutations and ledger-directed crash
  recovery.
- Normal reconciliation never overwrites a real user directory.
- Destructive cleanup verifies ownership and fails closed when content changed.
- Update checks preserve the original Git transport without writing to the
  persistent mirror.
- Status and dry-run operations are read-only.

## Development

```bash
make test
make test-e2e
go vet ./...
```

The end-to-end test uses only temporary local directories and a local skill; it
does not require network access or alter real agent directories.

The implementation specification is
[`docs/superpowers/specs/2026-08-13-global-skills-manager-design.md`](docs/superpowers/specs/2026-08-13-global-skills-manager-design.md).

## License

Apache-2.0
