# vhs tape: Contrast-validation swatch — DARK owned canvas (§16.5 lock-in gate).
#
# Drives the offline capture harness (cmd/capturetool) with the
# `contrast-validation` fixture in dark mode. The swatch is the task 1-9
# lock-in/bail validation surface: a labelled set of tint bands — bg.selection,
# bg.warning, bg.track and the separator/footer borders — each filled with its
# 1-9-pinned tint and carrying its §4.1 on-tint foreground text, rendered on the
# owned dark canvas (#0b0c14). It is NOT the production Sessions surface (those
# tint surfaces land in later phases); it validates the colour TOKENS before
# those phases invest in the surfaces (anti-sunk-cost). Like the sessions tapes
# this NEVER opens a tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/contrast-validation-dark.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/contrast-validation-dark.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the swatch fixture in dark mode.
# `go run` compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "go run ./cmd/capturetool --fixture contrast-validation --appearance dark"
Enter

# First paint: the swatch renders the four tint bands on the dark canvas. Give
# the compile + first render time to settle.
Sleep 4s

Screenshot "testdata/vhs/contrast-validation-dark.png"
