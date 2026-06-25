# vhs tape: Sessions — Flat list (current, pre-reskin baseline).
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-flat` fixture and screenshots the first painted frame. The harness
# imports Portal's real internal/tui and binds every tmux seam to an in-memory
# fake, so this NEVER opens a tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/sessions-flat.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/sessions-flat.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the deterministic fixture. `go run`
# compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "go run ./cmd/capturetool --fixture sessions-flat"
Enter

# First paint: the model fetches the fixture sessions on Init and renders the
# flat list. The 4s pad covers the `go run` compile (cold build cache) + first
# render — don't trim it or a cold-cache run flakes.
Sleep 4s

Screenshot "testdata/vhs/sessions-flat.png"
