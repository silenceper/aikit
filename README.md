# aikit

**Manage AI dev assets. Sync once, align everyone.**

[![CI](https://github.com/silenceper/aikit/actions/workflows/ci.yml/badge.svg)](https://github.com/silenceper/aikit/actions/workflows/ci.yml)
[![Release](https://github.com/silenceper/aikit/actions/workflows/release.yml/badge.svg)](https://github.com/silenceper/aikit/releases)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

aikit is a unified CLI for managing AI development assets — **Skills**, **Rules**, **MCP configs**, and **Commands** — across multiple AI coding IDEs. Collect and organize your favorite assets in a personal catalog, publish your own creations to share with the community, and declare everything in a single `.aikit.yaml` so every team member can reproduce the same AI-powered dev environment with one command.

## Why aikit?

Every AI coding IDE has its own way of handling rules and configurations:

| IDE            | Rules                             | MCP                  | Commands                |
| -------------- | --------------------------------- | -------------------- | ----------------------- |
| Cursor         | `.cursor/rules/*.mdc`             | `.cursor/mcp.json`   | `.cursor/commands/*.md` |
| Claude Code    | `CLAUDE.md`                       | `.mcp.json`          | `.claude/commands/*.md` |
| GitHub Copilot | `.github/copilot-instructions.md` | `.vscode/mcp.json`   | —                       |
| Windsurf       | `.windsurf/rules/*.md`            | `.windsurf/mcp.json` | —                       |
| Codex          | `AGENTS.md`                       | `.codex/config.toml` | —                       |

This creates three problems:

1. **Hard to share** — The same rule (e.g. "follow team coding standards") is stored in different formats across IDEs. You can't just copy files around.
2. **Hard to collaborate** — Rules accumulate on individual machines. New team members have no way to get them.
3. **Hard to reuse** — Great skills and configs from the community have no standard way to be collected and distributed.

**aikit solves this** with one config file (`.aikit.yaml`) and one sync command. Write rules once in a standard format, and aikit converts them to each IDE's native format automatically.

## Installation

### From Go

```bash
go install github.com/silenceper/aikit@latest
```

### From releases

Download the pre-built binary for your platform from the [Releases](https://github.com/silenceper/aikit/releases) page.

### From source

```bash
git clone https://github.com/silenceper/aikit.git
cd aikit
make install
```

## Usage Scenarios

### Scenario 1: Multi-IDE sync & team sharing via `.aikit.yaml`

The core workflow — add skills, rules, and MCP configs to your project, sync them to all IDEs, and share with your team by committing one file.

**Set up your project:**

```bash
cd my-project
aikit init
```

**Add assets (interactive or by flag):**

```bash
# Interactive — discover and select from a remote repo
aikit add vercel-labs/agent-skills

# Or specify exactly what you need
aikit add vercel-labs/agent-skills --skill code-review
aikit add your-org/shared-assets --rule api-conventions
aikit add your-org/shared-assets --mcp playwright
```

**Sync to all your IDEs at once:**

```bash
aikit sync
```

aikit reads `.aikit.yaml` and installs everything into each IDE's native format:

- Skills → installed to `.cursor/skills/`, `.claude/skills/`, `.codex/skills/`, etc. (symlink preferred, copy as fallback)
- Rules → `.mdc` (Cursor), `CLAUDE.md` (Claude Code), `AGENTS.md` (Codex), etc.
- MCP → `.cursor/mcp.json`, `.mcp.json`, `.codex/config.toml`, etc.
- Commands → `.cursor/commands/`, `.claude/commands/`, etc.

**Share with your team — commit `.aikit.yaml` to your repo:**

```bash
git add .aikit.yaml
git commit -m "add AI dev environment"
git push
```

Teammates working on the same project just run:

```bash
aikit sync    # Assets installed to all their IDEs
```

**Import another project's AI setup (interactive):**

Found a project or team repo with a great `.aikit.yaml`? Import it and **pick only the assets you need**:

```bash
# From a remote repo
aikit init --from your-org/reference-project
# → Shows all assets in that config
# → You interactively select which ones to include
# → Creates your local .aikit.yaml with only selected assets

# From a local file
aikit init --from ../other-project/.aikit.yaml

# Then sync to your IDEs
aikit sync
```

This makes it easy to bootstrap new projects from a team template — without blindly copying everything.

### Scenario 2: Publish & share your assets with the community

Created a useful skill or rule? Publish it to a remote Git repo so anyone can use it.

**Example: you wrote a skill in `.cursor/skills/deploy-checker/`**

```bash
# Publish to your public asset repo
aikit publish --remote your-name/ai-assets --skill deploy-checker
```

aikit copies the skill to the remote repo and pushes. Now anyone can use it:

```bash
# Others add your skill to their project
aikit add your-name/ai-assets --skill deploy-checker
aikit sync
```

You can also publish rules and commands:

```bash
aikit publish --remote your-name/ai-assets --rule api-conventions
```

### Scenario 3: Global catalog — collect, organize, reuse

The global catalog (`~/.aikit/catalog.yaml`) is your personal library of AI assets, collected from anywhere and reusable across all your projects.

**Discover and collect:**

```bash
# Found a great asset repo? Add it to your catalog (interactive selection)
aikit catalog add vercel-labs/agent-skills

# Or add a specific asset
aikit catalog add your-org/shared-assets --rule api-conventions
```

**Browse your collection:**

```bash
aikit catalog list
```

```
Skills:
  AI Tools
    - code-review — Automated code review assistant (source: vercel-labs/agent-skills)
    - vercel-deploy — Deploy to Vercel (source: vercel-labs/agent-skills)

Rules:
  Team Standards
    - api-conventions — API design conventions (source: your-org/shared-assets)
```

**Use in any project — pick from your catalog:**

```bash
cd new-project
aikit init

# Interactive: browse your catalog and select what this project needs
aikit add

aikit sync
```

**Manage your catalog:**

```bash
# Remove assets you no longer need (interactive)
aikit catalog remove

# Update all cached remote assets to latest
aikit catalog update

# Sync your catalog across multiple devices via a private Git repo
aikit catalog sync --remote git@github.com:you/my-aikit-catalog.git
```

## Asset Formats

### Skill (`SKILL.md`)

Compatible with the [Agent Skills](https://github.com/vercel-labs/agent-skills) ecosystem:

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
  name: coding-standards
  description: "Enforce team coding standards and best practices"
spec:
  content_file: content.md
  globs: ["**/*.go", "**/*.ts"]
  always_apply: false
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
    - codex
assets:
  skills:
    - source: vercel-labs/agent-skills
      name: code-review
  rules:
    - source: your-org/shared-assets
      name: api-conventions
  mcps:
    - source: your-org/shared-assets
      name: playwright
  commands:
    - source: your-org/shared-assets
      name: review
local_rules:
  - name: project-conventions
    content: "Follow our internal API naming conventions: use camelCase for JSON fields..."
    always_apply: true
```

## Command Reference

```
aikit
├── init [--from <source>]              # Initialize project (from remote repo or local file)
├── add [<source>] [flags]              # Add asset to project
├── remove [flags]                      # Remove asset from project
├── list                                # List project assets
├── sync [--target <agent>...]          # Sync assets to IDEs
├── publish --remote <repo> [flags]     # Publish local assets to remote repo
├── catalog
│   ├── add <source> [flags]            # Register assets to global catalog
│   ├── remove [flags]                  # Remove from global catalog
│   ├── list                            # List catalog entries
│   ├── update [<source>]              # Update cached remote assets
│   └── sync [--remote <repo>]          # Multi-device catalog sync
└── version                             # Print version info
```

All commands that accept `--skill/--rule/--mcp/--command` flags will fall back to **interactive selection** when no flags are provided.

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
