# vhs tape: Sessions — Help Modal (?), LIGHT owned canvas (§8.5 / §14.4 / §15.6).
#
# Mirror of sessions-help-modal.tape but drives cmd/capturetool with
# --appearance light so the help modal paints on the light mode-matched canvas
# (#e1e2e7). This is the deferred task-1-9 light eyeball punted to this surface
# (§15.6): a human confirms the two-column glyph/label wiring + header read
# correctly in light mode. Like the dark tape this NEVER opens a tmux server or
# touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/sessions-help-modal-light.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/sessions-help-modal-light.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the deterministic fixture in light
# mode. `go run` compiles + runs in one step; no tmux server, no bootstrap.
Type "go run ./cmd/capturetool --fixture sessions-flat --appearance light"
Enter

# First paint: the model fetches the fixture sessions on Init and renders the
# flat list on the light canvas. Give the compile + first render time to settle.
Sleep 4s

# Open the per-page ? help modal (§8.5) over the Sessions page.
Type "?"
Sleep 1s

Screenshot "testdata/vhs/sessions-help-modal-light.png"
