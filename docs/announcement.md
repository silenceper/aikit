<p align="center">
  <img src="images/logo.png" alt="aikit logo" width="120">
</p>

# Introducing aikit — Manage Your AI Coding Assets. Sync Once, Align Everyone.

If you're using AI-powered IDEs like Cursor, Claude Code, GitHub Copilot, Windsurf, or Codex, you've probably run into the same frustration: every IDE stores rules, skills, MCP configs, and commands in its own format, in its own directories. Your carefully crafted prompts and configurations are locked into one tool, scattered across machines, and impossible to share cleanly.

**[aikit](https://github.com/silenceper/aikit)** is an open-source CLI that solves this. It gives you one standard format for all your AI development assets and one command to sync them everywhere.

## The Problem

You wrote a great "code review" skill in Cursor. Now your teammate on Claude Code wants it. Another colleague is trying Codex. Someone new just joined the team with a blank IDE setup.

What do you do? Copy files around? Maintain parallel versions? Write a wiki page with manual instructions? None of these scale.

Meanwhile, the community is producing incredible skills and MCP configs — but there's no standard way to discover, collect, and distribute them across different tools.

## How aikit Works

### 1. Collect your favorite skills, rules, and MCP configs into a personal catalog

Think of it as a bookmark manager for AI dev assets. Found a great skill repo on GitHub? Add it to your catalog:

```bash
aikit catalog add anthropics/skills
# → Scans the repo, discovers 18 skills
# → Select the ones you want, assign a group
# → Saved to your personal catalog
```

Your catalog lives at `~/.aikit/catalog.yaml` and follows you across all projects. When starting a new project, just pick from what you've already collected:

```bash
aikit init   # interactive wizard reads from your catalog
```

And if you prefer a visual interface over the terminal, there's a built-in web UI:

```bash
aikit catalog ui
```

![AIKIT Catalog Manager](https://raw.githubusercontent.com/silenceper/aikit/main/docs/images/catalog-ui.png)

Enter any GitHub repo, Git URL, or local path — aikit auto-discovers all assets inside, and you select what to keep. Browse by type (Skills, Rules, MCPs, Commands), organize into groups, all from your browser.

### 2. Publish your local assets to share with the world

You've built a great skill in `.cursor/skills/deploy-checker/`, or a set of rules your team relies on. Why keep them locked on your machine?

aikit can scan your project's IDE directories, find assets you've created locally, and publish them to any Git repo in one command:

```bash
aikit publish --remote your-name/ai-assets
# → Discovers local skills, rules, and commands in your project
# → Interactive selection: pick what to publish
# → Pushes to your-name/ai-assets, organized by type
# → Automatically updates source references in .aikit.yaml and catalog
```

Or publish a specific asset directly:

```bash
aikit publish --remote your-name/ai-assets --skill deploy-checker
aikit publish --remote your-name/ai-assets --rule api-conventions
```

Once published, anyone in the world can use your assets:

```bash
# Others add your skill to their project
aikit add your-name/ai-assets --skill deploy-checker
aikit sync
```

The publish flow also handles bookkeeping for you — after publishing, `source: _local` references in your `.aikit.yaml` and catalog are automatically updated to the remote repo URL, so `aikit sync` on other machines can fetch them seamlessly.

### 3. Share your team's AI environment with a single file

This is where aikit really shines. Declare your project's AI assets in `.aikit.yaml`:

```yaml
project:
  name: my-service
  targets: [cursor, claude-code, codex]
assets:
  skills:
    - source: your-org/shared-assets
      name: code-review
    - source: your-org/shared-assets
      name: api-design-guide
  rules:
    - source: your-org/shared-assets
      name: coding-standards
  mcps:
    - source: your-org/shared-assets
      name: playwright
```

Commit this file to your repo. Now every team member — regardless of which IDE they use — runs one command:

```bash
aikit sync
```

aikit fetches the assets, converts them to each IDE's native format, and installs them. Cursor gets `.mdc` rule files, Claude Code gets `CLAUDE.md`, Copilot gets `copilot-instructions.md` — all from the same source of truth.

**New hire onboarding?** Clone the repo, run `aikit sync`. Done.
**Switching IDEs?** Run `aikit sync`. Your rules follow you.
**Updated a shared skill?** Team runs `aikit sync`. Everyone is aligned.

## A Typical Team Workflow

Here's how it all fits together in practice:

```
Alice (Tech Lead)                        Bob (New Team Member)
─────────────────                        ────────────────────

# Collect great assets from the community
aikit catalog add anthropics/skills
aikit catalog add vercel-labs/agent-skills

# Create a custom skill locally
# (writes deploy-checker in .cursor/skills/)

# Publish it to the team's shared repo
aikit publish --remote acme/ai-assets     
  --skill deploy-checker

# Set up the project config
aikit init
aikit add acme/ai-assets --rule coding-standards
aikit add acme/ai-assets --mcp playwright
git add .aikit.yaml && git push
                                         # Day 1: clone and sync
                                         git clone acme/my-service
                                         cd my-service
                                         aikit sync
                                         # → All skills, rules, MCPs installed
                                         #   into Cursor, Claude Code, etc.
                                         # → Ready to code with full AI setup
```

No wiki pages. No Slack messages asking "where's that rule file?" No manual copying between IDE config directories.

## Why This Matters

The AI coding tool ecosystem is fragmenting fast. New IDEs and agents appear every month, each with proprietary config formats. Without a standard layer, teams end up with:

- **Knowledge silos** — rules and prompts trapped in individual IDE setups
- **Inconsistent AI behavior** — each team member's IDE acts differently
- **Manual overhead** — maintaining parallel configs across tools
- **Lost work** — great skills created locally that never get shared

aikit is that standard layer. It's like `package.json` for your AI dev environment — version-controlled, shareable, and portable across every tool.

## Key Features

- **Multi-IDE sync** — One config, all IDEs. Supports Cursor, Claude Code, Copilot, Windsurf, Codex, and more.
- **Team sharing** — Commit `.aikit.yaml`, teammates run `aikit sync`. Zero manual setup.
- **Publish to remote** — Push your local skills, rules, and commands to any Git repo with `aikit publish`. Anyone can use them.
- **Personal catalog** — Collect skills and configs from anywhere, reuse across projects.
- **Web UI** — Visual catalog management with `aikit catalog ui`.
- **Standard asset format** — A unified spec for Rules, MCPs, and Commands (`asset.yaml`), plus native support for the [Agent Skills](https://docs.anthropic.com/en/docs/agents-and-tools/agent-skills/overview) `SKILL.md` format — so you can directly use skills from that ecosystem.
- **Single binary** — Go binary with embedded frontend. No runtime dependencies.

## Get Started

```bash
# Install via Homebrew(macOS / Linux)
brew tap silenceper/tap && brew install aikit

# Download the binary from the releases page
https://github.com/silenceper/aikit/releases

# Collect some skills
aikit catalog add anthropics/skills

# Browse your catalog visually
aikit catalog ui

# Publish your own skills
aikit publish --remote your-name/ai-assets

# Set up a project
cd my-project && aikit init

# Sync to all your IDEs
aikit sync
```

GitHub: **[github.com/silenceper/aikit](https://github.com/silenceper/aikit)**
