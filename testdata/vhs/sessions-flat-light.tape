# vhs tape: Sessions — Flat list, LIGHT owned canvas.
#
# Mirror of sessions-flat.tape, but drives the offline capture harness
# (cmd/capturetool) with --appearance light so the model paints the light
# mode-matched canvas (#e1e2e7) instead of the dark one (#0b0c14). Detection
# (task 1-7) is not landed yet, so the harness injects the light theme.Mode
# explicitly via the --appearance flag (cmd/capturetool resolveMode). Like the
# dark tape this NEVER opens a tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/sessions-flat-light.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/sessions-flat-light.gif"

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

Screenshot "testdata/vhs/sessions-flat-light.png"
