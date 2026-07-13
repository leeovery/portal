# vhs tape: Sessions — Multi-Select (active) — the §5 multi-select mode.
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-multi-select-active` fixture — the sessions-flat set opened directly
# in multi-select mode with three sessions marked (agentic-workflows-codify,
# fab-flowx-explore, designlab-web-r8suyU) and the cursor anchored on the marked
# row fab-flowx-explore — and screenshots the first painted frame: the violet
# `3 selected` banner + `esc cancel`, the `●` markers (incl. the banded cursor
# row), and the multi-select footer. The harness imports Portal's real
# internal/tui and binds every tmux seam to an in-memory fake, so this NEVER opens
# a tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/sessions-multi-select-active.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/sessions-multi-select-active.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the deterministic fixture. `go run`
# compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "go run ./cmd/capturetool --fixture sessions-multi-select-active"
Enter

# First paint: the model fetches the fixture sessions on Init, enters multi-select
# mode with the seeded marked set, and anchors the cursor on fab-flowx-explore. The
# 4s pad covers the `go run` compile (cold build cache) + first render — don't trim
# it or a cold-cache run flakes.
Sleep 4s

Screenshot "testdata/vhs/sessions-multi-select-active.png"
