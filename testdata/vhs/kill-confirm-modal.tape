# vhs tape: Kill Confirm Modal — MV reskin (§8.3 / §8.1).
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-flat` fixture, navigates the cursor to `aviva-proxy-qNyfEO` (the
# reference target — a 1-window session), opens the kill-confirm modal (k), and
# screenshots the result. The harness imports Portal's real internal/tui and binds
# every tmux seam to an in-memory fake (the fixture wires a Killer), so this NEVER
# opens a tmux server or touches ~/.config/portal.
#
# The capture verifies the §8.3 reskin: the hand-drawn single-tone joined panel
# (▲ Kill session? header / <name> · 1 window + consequence body / y kill · esc
# cancel footer), all in border.separator with joined ├───┤ dividers, centred on
# the cleared owned canvas. Compare against testdata/vhs/reference/
# kill-confirm-modal-mv.png.
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

# Move the cursor to `aviva-proxy-qNyfEO` (the 5th flat row, index 4) — the §8.3
# reference target, a 1-window session — so the modal shows `· 1 window`.
Down
Down
Down
Down
Sleep 500ms

# Open the kill-confirm modal on the selected session. This clears the page behind
# it to the owned canvas and centres the MV panel on it (§8.1 blank-screen layer).
Type "k"
Sleep 1s

Screenshot "testdata/vhs/kill-confirm-modal.png"
