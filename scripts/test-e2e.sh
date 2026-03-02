#!/usr/bin/env bash
#
# End-to-end integration test for aikit CLI.
# Uses git@github.com:silenceper/catalog-test.git as the test remote.
#
# Usage:
#   make test-e2e              # build + run
#   bash scripts/test-e2e.sh   # run directly (requires bin/aikit)
#
set -uo pipefail

# ---------- constants ----------
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
AIKIT="$PROJECT_ROOT/bin/aikit"
TEST_REMOTE="git@github.com:silenceper/catalog-test.git"
TEST_SOURCE="vercel-labs/agent-skills"
TEST_SKILL="vercel-deploy"

# ---------- temp workspace ----------
WORK_DIR="$(mktemp -d)"
TEST_PROJECT="$WORK_DIR/test-project"
export AIKIT_HOME="$WORK_DIR/aikit-home"

# ---------- helpers ----------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'
PASS_COUNT=0
FAIL_COUNT=0

pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
  echo -e "  ${GREEN}PASS${NC} $1"
}

fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  echo -e "  ${RED}FAIL${NC} $1"
  if [ -n "${2:-}" ]; then
    echo -e "  ${RED}      $2${NC}"
  fi
}

step() {
  echo ""
  echo -e "${YELLOW}=== $1 ===${NC}"
}

# run_cmd captures stdout+stderr and exit code without aborting the script.
run_cmd() {
  OUTPUT=$("$@" 2>&1) && CMD_RC=0 || CMD_RC=$?
}

cleanup() {
  echo ""
  echo "Cleaning up $WORK_DIR ..."
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

# ---------- setup ----------
mkdir -p "$AIKIT_HOME"
mkdir -p "$TEST_PROJECT"

# ---------- step 1: version ----------
step "Step 1: aikit version"
run_cmd "$AIKIT" version
if echo "$OUTPUT" | grep -q "aikit version"; then
  pass "version output correct"
else
  fail "version output unexpected" "$OUTPUT"
fi

# ---------- step 2: init ----------
step "Step 2: aikit init"
run_cmd "$AIKIT" init -C "$TEST_PROJECT"
if [ -f "$TEST_PROJECT/.aikit.yaml" ]; then
  pass ".aikit.yaml created"
else
  fail ".aikit.yaml not found" "$OUTPUT"
fi

# ---------- step 3: catalog add (remote, --skill) ----------
step "Step 3: aikit catalog add (remote)"
run_cmd "$AIKIT" catalog add "$TEST_SOURCE" --skill "$TEST_SKILL"
if [ "$CMD_RC" -eq 0 ] && echo "$OUTPUT" | grep -qi "registered\|added"; then
  pass "catalog add registered skill"
else
  fail "catalog add failed (rc=$CMD_RC)" "$OUTPUT"
fi

# ---------- step 4: catalog list ----------
step "Step 4: aikit catalog list"
run_cmd "$AIKIT" catalog list
if echo "$OUTPUT" | grep -q "$TEST_SKILL"; then
  pass "catalog list contains $TEST_SKILL"
else
  fail "catalog list missing $TEST_SKILL" "$OUTPUT"
fi

# ---------- step 5: catalog remove ----------
step "Step 5b: aikit catalog remove"
run_cmd "$AIKIT" catalog remove --skill "$TEST_SKILL"
if [ "$CMD_RC" -eq 0 ] && echo "$OUTPUT" | grep -qi "removed"; then
  pass "catalog remove skill"
else
  fail "catalog remove failed (rc=$CMD_RC)" "$OUTPUT"
fi
# Re-add it for later tests
run_cmd "$AIKIT" catalog add "$TEST_SOURCE" --skill "$TEST_SKILL"

# ---------- step 6: add assets to project (all types) ----------
step "Step 6: aikit add (all asset types)"

# 6a. add skill from remote
run_cmd "$AIKIT" add "$TEST_SOURCE" --skill "$TEST_SKILL" -C "$TEST_PROJECT"
if [ "$CMD_RC" -eq 0 ] && echo "$OUTPUT" | grep -qi "added"; then
  pass "add skill from remote"
else
  fail "add skill failed (rc=$CMD_RC)" "$OUTPUT"
fi

# 6b. add same skill again (dedup test)
run_cmd "$AIKIT" add "$TEST_SOURCE" --skill "$TEST_SKILL" -C "$TEST_PROJECT"
if [ "$CMD_RC" -eq 0 ] && echo "$OUTPUT" | grep -qi "skipped\|already"; then
  pass "add skill dedup (skipped)"
else
  fail "add skill dedup failed (rc=$CMD_RC)" "$OUTPUT"
fi

# 6c. add rule from remote
run_cmd "$AIKIT" add "$TEST_SOURCE" --rule e2e-test-rule -C "$TEST_PROJECT"
if [ "$CMD_RC" -eq 0 ] && echo "$OUTPUT" | grep -qi "added"; then
  pass "add rule from remote"
else
  fail "add rule failed (rc=$CMD_RC)" "$OUTPUT"
fi

# 6d. add mcp from remote
run_cmd "$AIKIT" add "$TEST_SOURCE" --mcp e2e-test-mcp -C "$TEST_PROJECT"
if [ "$CMD_RC" -eq 0 ] && echo "$OUTPUT" | grep -qi "added"; then
  pass "add mcp from remote"
else
  fail "add mcp failed (rc=$CMD_RC)" "$OUTPUT"
fi

# 6e. add command from remote
run_cmd "$AIKIT" add "$TEST_SOURCE" --command e2e-test-command -C "$TEST_PROJECT"
if [ "$CMD_RC" -eq 0 ] && echo "$OUTPUT" | grep -qi "added"; then
  pass "add command from remote"
else
  fail "add command failed (rc=$CMD_RC)" "$OUTPUT"
fi

# ---------- step 7: list project assets (verify all types) ----------
step "Step 7: aikit list (all asset types)"
run_cmd "$AIKIT" list -C "$TEST_PROJECT"
if echo "$OUTPUT" | grep -q "$TEST_SKILL"; then
  pass "list shows skill $TEST_SKILL"
else
  fail "list missing skill $TEST_SKILL" "$OUTPUT"
fi
if echo "$OUTPUT" | grep -q "e2e-test-rule"; then
  pass "list shows rule e2e-test-rule"
else
  fail "list missing rule" "$OUTPUT"
fi
if echo "$OUTPUT" | grep -q "e2e-test-mcp"; then
  pass "list shows mcp e2e-test-mcp"
else
  fail "list missing mcp" "$OUTPUT"
fi
if echo "$OUTPUT" | grep -q "e2e-test-command"; then
  pass "list shows command e2e-test-command"
else
  fail "list missing command" "$OUTPUT"
fi

# ---------- step 8: sync to cursor (non-interactive) ----------
step "Step 8: aikit sync --target cursor"
mkdir -p "$TEST_PROJECT/.cursor"
run_cmd "$AIKIT" sync --target cursor -C "$TEST_PROJECT"
if [ "$CMD_RC" -eq 0 ]; then
  pass "sync to cursor"
else
  fail "sync failed (rc=$CMD_RC)" "$OUTPUT"
fi
if [ -d "$TEST_PROJECT/.cursor/skills/$TEST_SKILL" ] || [ -L "$TEST_PROJECT/.cursor/skills/$TEST_SKILL" ]; then
  pass "skill directory installed"
else
  fail "skill directory not found in .cursor/skills/" ""
fi

# ---------- step 8b: local_rules sync test ----------
step "Step 8b: local_rules sync"
# Create a test asset repo with rule (asset.yaml + content.md)
ASSET_REPO="$WORK_DIR/asset-repo"
mkdir -p "$ASSET_REPO/rules/test-rule"
cat > "$ASSET_REPO/rules/test-rule/asset.yaml" << 'YAMLEOF'
kind: rule
metadata:
  name: test-rule
  description: A test rule for e2e
spec:
  content_file: content.md
  always_apply: true
YAMLEOF
cat > "$ASSET_REPO/rules/test-rule/content.md" << 'MDEOF'
Always respond in Chinese.
MDEOF

# Create command asset
mkdir -p "$ASSET_REPO/commands/test-cmd"
cat > "$ASSET_REPO/commands/test-cmd/asset.yaml" << 'YAMLEOF'
kind: command
metadata:
  name: test-cmd
  description: A test command
spec:
  content_file: content.md
YAMLEOF
cat > "$ASSET_REPO/commands/test-cmd/content.md" << 'MDEOF'
Review this code for security issues.
MDEOF

# Register local rule + command to catalog
run_cmd "$AIKIT" catalog add "$ASSET_REPO" --rule test-rule
if [ "$CMD_RC" -eq 0 ] && echo "$OUTPUT" | grep -qi "registered"; then
  pass "catalog add local rule"
else
  fail "catalog add local rule failed (rc=$CMD_RC)" "$OUTPUT"
fi

run_cmd "$AIKIT" catalog add "$ASSET_REPO" --command test-cmd
if [ "$CMD_RC" -eq 0 ] && echo "$OUTPUT" | grep -qi "registered"; then
  pass "catalog add local command"
else
  fail "catalog add local command failed (rc=$CMD_RC)" "$OUTPUT"
fi

# Add local_rules to .aikit.yaml directly for testing
cat >> "$TEST_PROJECT/.aikit.yaml" << 'LREOF'
local_rules:
    - name: respond-chinese
      content: "Always respond in Chinese"
      always_apply: true
LREOF

# Re-add the skill for sync test
run_cmd "$AIKIT" add "$TEST_SOURCE" --skill "$TEST_SKILL" -C "$TEST_PROJECT"
# Sync again to test rule and local_rules output
run_cmd "$AIKIT" sync --target cursor -C "$TEST_PROJECT"
if [ "$CMD_RC" -eq 0 ] && echo "$OUTPUT" | grep -qi "rule"; then
  pass "sync handles rules/local_rules"
else
  fail "sync rule handling unexpected (rc=$CMD_RC)" "$OUTPUT"
fi

# ---------- step 9: sync dry-run (verify all types counted) ----------
step "Step 9: aikit sync --dry-run"
run_cmd "$AIKIT" sync --target cursor --dry-run -C "$TEST_PROJECT"
if [ "$CMD_RC" -eq 0 ] && echo "$OUTPUT" | grep -q "Skills:.*1"; then
  pass "dry-run shows skill count"
else
  fail "dry-run skill count unexpected (rc=$CMD_RC)" "$OUTPUT"
fi
if echo "$OUTPUT" | grep -q "Rules:.*1"; then
  pass "dry-run shows rule count"
else
  fail "dry-run rule count unexpected" "$OUTPUT"
fi
if echo "$OUTPUT" | grep -q "MCPs:.*1"; then
  pass "dry-run shows mcp count"
else
  fail "dry-run mcp count unexpected" "$OUTPUT"
fi
if echo "$OUTPUT" | grep -q "Commands:.*1"; then
  pass "dry-run shows command count"
else
  fail "dry-run command count unexpected" "$OUTPUT"
fi

# ---------- step 10: remove assets (all types) ----------
step "Step 10: aikit remove (all asset types)"

# 10a. remove skill (verify feedback)
run_cmd "$AIKIT" remove --skill "$TEST_SKILL" -C "$TEST_PROJECT"
if [ "$CMD_RC" -eq 0 ] && echo "$OUTPUT" | grep -qi "removed"; then
  pass "remove skill with feedback"
else
  fail "remove skill failed (rc=$CMD_RC)" "$OUTPUT"
fi

# 10b. remove rule
run_cmd "$AIKIT" remove --rule e2e-test-rule -C "$TEST_PROJECT"
if [ "$CMD_RC" -eq 0 ]; then
  pass "remove rule exit ok"
else
  fail "remove rule failed (rc=$CMD_RC)" "$OUTPUT"
fi

# 10c. remove mcp
run_cmd "$AIKIT" remove --mcp e2e-test-mcp -C "$TEST_PROJECT"
if [ "$CMD_RC" -eq 0 ]; then
  pass "remove mcp exit ok"
else
  fail "remove mcp failed (rc=$CMD_RC)" "$OUTPUT"
fi

# 10d. remove command
run_cmd "$AIKIT" remove --command e2e-test-command -C "$TEST_PROJECT"
if [ "$CMD_RC" -eq 0 ]; then
  pass "remove command exit ok"
else
  fail "remove command failed (rc=$CMD_RC)" "$OUTPUT"
fi

# Verify all removed
run_cmd "$AIKIT" list -C "$TEST_PROJECT"
if ! echo "$OUTPUT" | grep -q "skill.*$TEST_SKILL"; then
  pass "skill gone after remove"
else
  fail "skill still present" "$OUTPUT"
fi
if ! echo "$OUTPUT" | grep -q "rule.*e2e-test-rule"; then
  pass "rule gone after remove"
else
  fail "rule still present" "$OUTPUT"
fi
if ! echo "$OUTPUT" | grep -q "mcp.*e2e-test-mcp"; then
  pass "mcp gone after remove"
else
  fail "mcp still present" "$OUTPUT"
fi
if ! echo "$OUTPUT" | grep -q "command.*e2e-test-command"; then
  pass "command gone after remove"
else
  fail "command still present" "$OUTPUT"
fi

# ---------- step 11: publish (non-interactive, --skill) ----------
step "Step 11: aikit publish --skill"
FAKE_SKILL_DIR="$TEST_PROJECT/.cursor/skills/e2e-test-skill"
mkdir -p "$FAKE_SKILL_DIR"
cat > "$FAKE_SKILL_DIR/SKILL.md" << 'SKILLEOF'
---
name: e2e-test-skill
description: Skill created by e2e test
---
# E2E Test Skill

This is a test skill for integration testing.
SKILLEOF

run_cmd "$AIKIT" publish --remote "$TEST_REMOTE" --skill e2e-test-skill -C "$TEST_PROJECT"
if [ "$CMD_RC" -eq 0 ] && echo "$OUTPUT" | grep -qi "published\|up to date"; then
  pass "publish skill to remote"
else
  fail "publish failed (rc=$CMD_RC)" "$OUTPUT"
fi

# ---------- step 12: catalog sync ----------
step "Step 12: aikit catalog sync"
run_cmd "$AIKIT" catalog sync --remote "$TEST_REMOTE"
if [ "$CMD_RC" -eq 0 ] && echo "$OUTPUT" | grep -qi "synced successfully"; then
  pass "catalog sync to remote"
else
  fail "catalog sync failed (rc=$CMD_RC)" "$OUTPUT"
fi

# Run again without --remote to test remembered remote
run_cmd "$AIKIT" catalog sync
if [ "$CMD_RC" -eq 0 ] && echo "$OUTPUT" | grep -qi "synced successfully\|no changes"; then
  pass "catalog sync (remembered remote)"
else
  fail "catalog sync second run failed (rc=$CMD_RC)" "$OUTPUT"
fi

# ---------- summary ----------
echo ""
echo "=============================="
TOTAL=$((PASS_COUNT + FAIL_COUNT))
echo -e "Results: ${GREEN}$PASS_COUNT passed${NC}, ${RED}$FAIL_COUNT failed${NC} / $TOTAL total"
echo "=============================="

if [ "$FAIL_COUNT" -gt 0 ]; then
  exit 1
fi
