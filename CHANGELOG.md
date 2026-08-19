# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
for published releases. During the alpha period, commands, configuration, and
recovery metadata may still change between releases.

## [Unreleased]

### Added

- A full-screen, mouse-capable TUI for Library, Overview, Workspaces, Presets,
  and Configuration workflows.
- Local and Git skill discovery, including GitHub shorthand and exact
  `skills.sh` source URLs.
- Global, project-common, and project-Agent bindings with reusable presets.
- Offline startup inventory, explicit import/adopt flows, and legacy migration.
- Atomic library mutations, authenticated link cleanup, and explicit recovery
  review for interrupted operations.
- Structured refs and cached remote update checks.

### Changed

- The project is now Skills-only and uses `$AIKIT_HOME/config.yaml` as its
  global ledger instead of legacy catalog or project-local ledger formats.

## [0.0.1-alpha.2] - 2026-03-02

### Changed

- Improved initial project documentation.

## [0.0.1-alpha.1] - 2026-03-02

### Added

- Initial alpha release with the foundational CLI, configuration, and Codex
  skill support.

[Unreleased]: https://github.com/silenceper/aikit/compare/v0.0.1-alpha.2...HEAD
[0.0.1-alpha.2]: https://github.com/silenceper/aikit/compare/v0.0.1-alpha.1...v0.0.1-alpha.2
[0.0.1-alpha.1]: https://github.com/silenceper/aikit/releases/tag/v0.0.1-alpha.1
