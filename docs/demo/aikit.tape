Output docs/assets/aikit-demo.gif

# Requires VHS >= 0.9.0 for Wait+Screen. Last rendered with Go 1.25.0,
# VHS 0.11.0, and Menlo on macOS. Run docs/demo/render.sh to reproduce it.

Require bash
Require dscl
Require ffmpeg
Require go
Require ttyd

Set Shell "bash"
Set FontFamily "Menlo"
Set FontSize 18
Set Width 1100
Set Height 660
Set Padding 16
Set WindowBar Colorful
Set Theme "Catppuccin Mocha"
Set Framerate 12
Set TypingSpeed 45ms
Set CursorBlink false
Set WaitTimeout 90s
Env HOME "/tmp/aikit-readme-demo/home"

# Build and seed entirely below the marker-owned demo root.
Hide
Type `set -euo pipefail; REAL_HOME="$(/usr/bin/dscl . -read "/Users/$(/usr/bin/id -un)" NFSHomeDirectory | /usr/bin/awk '{print $2}')"; DEMO_ROOT="/tmp/aikit-readme-demo"; export REAL_HOME DEMO_ROOT; cleanup_demo() { if [[ -f "$DEMO_ROOT/.aikit-readme-demo" ]]; then docs/demo/seed.sh clean "$DEMO_ROOT" "$REAL_HOME"; fi; }; trap cleanup_demo EXIT; docs/demo/seed.sh prepare "$DEMO_ROOT" "$REAL_HOME"; export HOME="$DEMO_ROOT/home" AIKIT_HOME="$DEMO_ROOT/aikit" GOCACHE="$DEMO_ROOT/cache/go-build" GOMODCACHE="$DEMO_ROOT/cache/go-mod" GOPATH="$DEMO_ROOT/cache/gopath" XDG_CACHE_HOME="$DEMO_ROOT/cache/xdg" TERM="xterm-256color" COLORTERM="truecolor"; export PATH="$DEMO_ROOT/bin:/opt/homebrew/bin:/usr/bin:/bin"; go build -o "$DEMO_ROOT/bin/aikit" .; docs/demo/seed.sh verify "$DEMO_ROOT" "$REAL_HOME"; PS1='$ '; clear`
Enter
Wait+Line /\$/
Show

# First run: discover three unmanaged local Skills.
Type "aikit"
Enter
Wait+Screen /Update check complete/
Sleep 2.5s

# Select and import all local Skills after reviewing the exact plan.
Right
Space
Down
Space
Down
Space
Sleep 1s
Enter
Wait+Screen /Review the exact migration plan/
Sleep 2s
Enter
Wait+Screen /Same content/
Sleep 1s

# Search the Library and inspect the imported release Skill.
Type "2"
Wait+Screen /release-checklist.*Managed/
Type "/"
Wait+Screen /Search:/
Type@70ms "release"
Enter
Wait+Screen /Filter: release/
Sleep 1s
Enter
Wait+Screen /Skill detail loaded/
Sleep 2s
Space
Sleep 500ms
Escape
Wait+Screen /1 selected/

# Clear the search, then enable the selected Skill for Global / codex.
Type "/"
Wait+Screen /Search: release/
Backspace@60ms 7
Enter
Wait+Screen /Library skills/
Type "e"
Wait+Screen /Choose an exact target scope/
Down@180ms 2
Enter
Wait+Screen /choose Apply to review/
Enter
Wait+Screen /Review the exact atomic batch/
Sleep 2s
Enter
Wait+Screen /Last library batch/
Sleep 400ms

# Finish on an explicit view of the new Codex binding.
Ctrl+P
Wait+Screen /Command palette/
Type@70ms "agents"
Enter
Wait+Screen /Workspace scopes/
Down@180ms 2
Enter
Wait+Screen /release-checklist.*Direct/
Sleep 4s

# Assert the real final state before marker-guarded cleanup.
Hide
Ctrl+Q
Sleep 1s
Type `docs/demo/seed.sh verify-result "$DEMO_ROOT" "$REAL_HOME"; docs/demo/seed.sh clean "$DEMO_ROOT" "$REAL_HOME"; trap - EXIT`
Enter
Wait+Line /\$/
