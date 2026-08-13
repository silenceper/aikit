#!/usr/bin/env bash
# Local, network-free end-to-end smoke test for the skills manager.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
AIKIT="$PROJECT_ROOT/bin/aikit"
WORK_DIR="$(mktemp -d)"

cleanup() {
  case "$WORK_DIR" in
    /tmp/*|/private/tmp/*|/var/folders/*|/private/var/folders/*) rm -rf "$WORK_DIR" ;;
    *) printf 'refusing to clean unexpected path: %s\n' "$WORK_DIR" >&2 ;;
  esac
}
trap cleanup EXIT

export HOME="$WORK_DIR/home"
export AIKIT_HOME="$HOME/.aikit"
SKILL_DIR="$WORK_DIR/demo"
PROJECT_DIR="$WORK_DIR/project"

mkdir -p "$HOME" "$SKILL_DIR" "$PROJECT_DIR"
printf '%s\n' '---' 'name: demo' 'description: local e2e skill' '---' '# Demo' > "$SKILL_DIR/SKILL.md"

if [ ! -x "$AIKIT" ]; then
  printf 'missing %s; run make build first\n' "$AIKIT" >&2
  exit 1
fi

"$AIKIT" version
"$AIKIT" add "$SKILL_DIR" --agent cursor
"$AIKIT" list --json | grep -q 'local/demo'
test -L "$HOME/.cursor/skills/demo"
test -f "$AIKIT_HOME/library/skills/local/demo/SKILL.md"

"$AIKIT" preset create essentials --skill local/demo
"$AIKIT" enable --preset essentials --agent claude-code
test -L "$HOME/.claude/skills/demo"

"$AIKIT" project add "$PROJECT_DIR" --name demo-project --agent codex
"$AIKIT" enable local/demo --project demo-project --agent codex
test -L "$PROJECT_DIR/.codex/skills/demo"

"$AIKIT" status --offline
"$AIKIT" sync --dry-run
"$AIKIT" sync

"$AIKIT" disable local/demo --project demo-project --agent codex
"$AIKIT" disable --preset essentials --agent claude-code
"$AIKIT" preset remove essentials
"$AIKIT" disable local/demo --agent cursor
"$AIKIT" project remove demo-project --yes
"$AIKIT" remove local/demo

"$AIKIT" list --json | grep -q 'local/demo' && {
  printf 'removed skill is still present\n' >&2
  exit 1
}

printf 'aikit local e2e: PASS\n'
