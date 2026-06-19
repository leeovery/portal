# vhs tape: Sessions — Flat list under NO_COLOR (the §2.5 colourless carve-out).
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-flat` fixture, but launches with NO_COLOR=1 set inline so the harness
# resolves the colourless native-bg render path: no painted canvas, no colour,
# state carried by glyphs (● attached / ▌ selector / spaced headers) + bold/dim.
# The harness imports Portal's real internal/tui and binds every tmux seam to an
# in-memory fake, so this NEVER opens a tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/sessions-flat-nocolor.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/sessions-flat-nocolor.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the deterministic fixture, with
# NO_COLOR=1 set inline so the colourless carve-out path renders. `go run`
# compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "NO_COLOR=1 go run ./cmd/capturetool --fixture sessions-flat"
Enter

# First paint: the model fetches the fixture sessions on Init and renders the
# flat list colourless on the terminal's native bg. Give the compile + first
# render time to settle.
Sleep 4s

Screenshot "testdata/vhs/sessions-flat-nocolor.png"
