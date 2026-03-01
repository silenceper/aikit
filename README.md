# aikit

**Manage AI dev assets. Sync once, align everyone.**

[![CI](https://github.com/silenceper/aikit/actions/workflows/ci.yml/badge.svg)](https://github.com/silenceper/aikit/actions/workflows/ci.yml)
[![Release](https://github.com/silenceper/aikit/actions/workflows/release.yml/badge.svg)](https://github.com/silenceper/aikit/releases)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

aikit is a unified CLI for managing AI development assets — **Skills**, **Rules**, **MCP configs**, and **Commands** — across multiple AI coding IDEs. Declare everything in a single `.aikit.yaml`, commit it with your project, and let every team member reproduce the same AI-powered dev environment with one command.

## Why aikit?

Every AI coding IDE has its own way of handling rules and configurations:

| IDE            | Rules                             | MCP                | Commands                |
| -------------- | --------------------------------- | ------------------ | ----------------------- |
| Cursor         | `.cursor/rules/*.mdc`             | `.cursor/mcp.json` | `.cursor/commands/*.md` |
| Claude Code    | `CLAUDE.md`                       | `.mcp.json`        | `.claude/commands/*.md` |
| GitHub Copilot | `.github/copilot-instructions.md` | —                  | —                       |
| Windsurf       | `.windsurf/rules/*.md`            | —                  | —                       |

This creates three problems:

1. **Hard to share** — The same rule ("respond in Chinese") looks different in every IDE. You can't just copy files around.
2. **Hard to collaborate** — Rules accumulate on individual machines. New team members have no way to get them.
3. **Hard to reuse** — Community best practices have no standard way to be collected and distributed.

**aikit solves this** with one config file (`.aikit.yaml`) and one sync command. Write rules once in a standard format, and aikit converts them to each IDE's native format automatically.

## Features

- **Multi-IDE sync** — Cursor, Claude Code, GitHub Copilot, Windsurf (more coming)
- **4 asset types** — Skills, Rules, MCP configs, Commands
- **Standard format** — `asset.yaml` for rules/MCP/commands, `SKILL.md` for skills (compatible with [Agent Skills](https://github.com/nicepkg/agent-skills) ecosystem)
- **Global catalog** — Personal asset collection at `~/.aikit/catalog.yaml`, reusable across projects
- **Team sharing** — Commit `.aikit.yaml` to Git, teammates run `aikit sync` to get everything
- **Multi-device sync** — Sync your global catalog to a private Git repo with `aikit catalog sync`
- **Interactive TUI** — All commands support interactive selection when no flags are specified
- **Publish** — Push local assets to a public remote repo with `aikit publish`

## Installation

### From Go

```bash
go install github.com/silenceper/aikit@latest
```

### From source

```bash
git clone https://github.com/silenceper/aikit.git
cd aikit
make install
```

### From releases

Download the pre-built binary for your platform from the [Releases](https://github.com/silenceper/aikit/releases) page.

## Quick Start

### 1. Initialize a project

```bash
cd your-project
aikit init
```

This creates `.aikit.yaml` in your project directory.

### 2. Add assets from a remote repository

```bash
# Add a specific skill
aikit add vercel-labs/agent-skills --skill vercel-deploy

# Add a rule
aikit add silenceper/ai-assets --rule respond-in-chinese

# Add an MCP config
aikit add silenceper/ai-assets --mcp playwright

# Or run without flags for interactive selection
aikit add silenceper/ai-assets
```

### 3. Sync to your IDEs

```bash
# Interactive — select which IDEs to sync to
aikit sync

# Or specify targets directly
aikit sync --target cursor --target claude-code
```

aikit reads `.aikit.yaml`, fetches assets from remote sources, and installs them into each IDE's native format:

- Skills → symlinked to `.cursor/skills/`, `.claude/skills/`, etc.
- Rules → converted to `.mdc` (Cursor), merged into `CLAUDE.md` (Claude Code), etc.
- MCP → merged into `.cursor/mcp.json`, `.mcp.json`, etc.
- Commands → written to `.cursor/commands/`, `.claude/commands/`, etc.

### 4. Commit and share

```bash
git add .aikit.yaml
git commit -m "add AI dev assets"
```

Your teammates clone the repo and run:

```bash
aikit sync
```

Done — everyone has the same AI rules, skills, and configs.

## Global Catalog

The global catalog (`~/.aikit/catalog.yaml`) is your personal collection of assets, reusable across all projects.

```bash
# Register assets from a remote repo
aikit catalog add silenceper/ai-assets

# List your catalog
aikit catalog list

# Add from catalog to current project (interactive)
aikit add

# Remove from catalog (interactive)
aikit catalog remove

# Update cached remote assets
aikit catalog update

# Sync catalog across devices via a private Git repo
aikit catalog sync --remote git@github.com:you/my-aikit-catalog.git
```

## Asset Formats

### Skill (`SKILL.md`)

Compatible with the [Agent Skills](https://github.com/nicepkg/agent-skills) ecosystem:

```markdown
---
name: code-review
description: Automated code review assistant
---
# Code Review

Review code for bugs, security issues, and style violations.
```

### Rule (`asset.yaml`)

```yaml
kind: rule
metadata:
  name: respond-in-chinese
  description: "Always respond in Chinese"
spec:
  content_file: content.md
  always_apply: true
```

### MCP (`asset.yaml`)

```yaml
kind: mcp
metadata:
  name: playwright
  description: "Browser automation via Playwright"
spec:
  transport: stdio
  command: "npx"
  args: ["-y", "@anthropic/mcp-playwright"]
```

### Command (`asset.yaml`)

```yaml
kind: command
metadata:
  name: review
  description: "Security-focused code review"
spec:
  content_file: content.md
```

## `.aikit.yaml` Example

```yaml
project:
  name: my-app
  targets:
    - cursor
    - claude-code
assets:
  skills:
    - source: vercel-labs/agent-skills
      name: vercel-deploy
  rules:
    - source: silenceper/ai-assets
      name: respond-in-chinese
  mcps:
    - source: silenceper/ai-assets
      name: playwright
  commands:
    - source: silenceper/ai-assets
      name: review
local_rules:
  - name: project-conventions
    content: "Follow our team coding standards..."
    always_apply: true
```

## Command Reference

```
aikit
├── init [--from <source>]              # Initialize project (or import from remote)
├── add [<source>] [flags]              # Add asset to project
├── remove [flags]                      # Remove asset from project
├── list                                # List project assets
├── sync [--target <agent>...]          # Sync assets to IDEs
├── publish --remote <repo> [flags]     # Publish local assets to remote repo
├── catalog
│   ├── add <source> [flags]            # Register assets to global catalog
│   ├── remove [flags]                  # Remove from global catalog
│   ├── list                            # List catalog entries
│   ├── update [<source>]               # Update cached remote assets
│   └── sync [--remote <repo>]          # Multi-device catalog sync
└── version                             # Print version info
```

All commands that accept `--skill/--rule/--mcp/--command` flags will fall back to interactive selection when no flags are provided.

## Development

```bash
# Build
make build

# Install to $GOPATH/bin
make install

# Run tests
make test

# Run end-to-end tests
make test-e2e
```

## Design

For detailed design documentation (in Chinese): [docs/design-zh.md](docs/design-zh.md)

## License

[Apache License 2.0](LICENSE)
