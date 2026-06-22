# vhs tape: Rename Modal — MV reskin, LIGHT owned canvas (§8.4 / §8.1 / §15.6).
#
# Mirror of rename-modal.tape but drives cmd/capturetool with --appearance light
# so the rename modal paints on the light mode-matched canvas (#e1e2e7). A human
# confirms the single-tone joined panel + the accent.violet NEW NAME label + the
# violet-outlined input box + violet block cursor + accent.blue footer glyphs read
# correctly in light mode (the §15.6 eyeball — there is NO light Paper reference for
# rename, so this is a legibility/structure confirmation, not a frame compare). Like
# the dark tape this NEVER opens a tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/rename-modal-light.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/rename-modal-light.gif"

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

# Move the cursor to `aviva-proxy-qNyfEO` (the 5th flat row, index 4) — the §8.4
# reference target.
Down
Down
Down
Down
Sleep 500ms

# Open the rename modal on the selected session. The input pre-fills with the old
# name; this clears the page behind it to the owned canvas and centres the MV panel
# on it (§8.1 blank-screen layer).
Type "r"
Sleep 500ms

# Clear the pre-filled name and type the reference value `aviva-proxy`.
Ctrl+U
Type "aviva-proxy"
Sleep 1s

Screenshot "testdata/vhs/rename-modal-light.png"
