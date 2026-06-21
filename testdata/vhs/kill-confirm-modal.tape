# vhs tape: Kill Confirm Modal — blank-screen modal layer (§8.1/13.5).
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-flat` fixture, opens the kill-confirm modal on the selected session
# (k), and screenshots the result. The harness imports Portal's real internal/tui
# and binds every tmux seam to an in-memory fake (the fixture wires a Killer), so
# this NEVER opens a tmux server or touches ~/.config/portal.
#
# The visible effect of the 3-1 plumbing task is the BACKDROP: the page behind the
# panel is cleared to the owned canvas (a flat mode-matched fill — no PORTAL
# header, no separator, no list rows, no dimmed overlay) with the centred panel on
# it. The panel's own styling/copy is a later task (3-5); this capture verifies the
# cleared canvas + centring only (compare against the Kill Confirm Modal (MV)
# reference for the blank-canvas backdrop).
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/kill-confirm-modal.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/kill-confirm-modal.gif"

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

# First paint: the model fetches the fixture sessions on Init and renders the flat
# list. Give the compile + first render time to settle before opening the modal.
Sleep 4s

# Open the kill-confirm modal on the selected session. This clears the page behind
# it to the owned canvas and centres the panel on it (the 3-1 blank-screen layer).
Type "k"
Sleep 1s

Screenshot "testdata/vhs/kill-confirm-modal.png"
