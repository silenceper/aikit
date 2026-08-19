# Support

aikit is maintained as an open-source project. Support is provided on a
best-effort basis and there is no guaranteed response or resolution time.

## Where to ask

- **Reproducible bug:** open a bug report using the GitHub issue template.
- **Feature or workflow proposal:** open a feature request and explain the user
  problem before proposing implementation details.
- **Documentation problem:** use the documentation issue template.
- **Security vulnerability:** follow [SECURITY.md](SECURITY.md) and report it
  privately. Do not open a public issue.
- **Contribution question:** open a draft pull request or a focused issue after
  reading [CONTRIBUTING.md](CONTRIBUTING.md).

Repository issues: <https://github.com/silenceper/aikit/issues>

## Before opening a bug

Run these commands and include sanitized output where relevant:

```bash
aikit version
aikit status --offline
aikit sync --dry-run
```

Also include the operating system, terminal, installation method, configured
agents, whether the problem is global or project-specific, and exact
reproduction steps. Remove credentials, private repository URLs, and personal
paths from logs and configuration.

## Scope

The current release manages Skills for the supported agents documented in the
README. Rules, MCP configuration, command packs, a Web UI, cross-machine
synchronization, and enterprise support are outside the current scope.

Questions about third-party agents, Git hosts, package managers, or terminal
behavior may need to be reproduced in the upstream project before aikit can
address them.
