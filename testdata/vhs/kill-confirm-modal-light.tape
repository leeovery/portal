# vhs tape: Kill Confirm Modal — MV reskin, LIGHT owned canvas (§8.3 / §8.1 / §15.6).
#
# Mirror of kill-confirm-modal.tape but drives cmd/capturetool with
# --appearance light so the kill modal paints on the light mode-matched canvas
# (#e1e2e7). A human confirms the single-tone joined panel + the state.red header/
# name + accent.blue footer glyphs read correctly in light mode. Like the dark tape
# this NEVER opens a tmux server or touches ~/.config/portal.
#
# Compare against testdata/vhs/reference/kill-confirm-modal-light.png.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/kill-confirm-modal-light.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/kill-confirm-modal-light.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the deterministic fixture in light mode.
# `go run` compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "go run ./cmd/capturetool --fixture sessions-flat --appearance light"
Enter

# First paint: the model fetches the fixture sessions on Init and renders the flat
# list on the light canvas. Give the compile + first render time to settle.
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

Screenshot "testdata/vhs/kill-confirm-modal-light.png"
