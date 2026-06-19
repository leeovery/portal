# vhs tape: Contrast-validation swatch — LIGHT owned canvas (§16.5 lock-in gate).
#
# Mirror of contrast-validation-dark.tape, but drives the swatch with
# --appearance light so the model paints the light mode-matched canvas (#e1e2e7)
# and resolves the LIGHT tint variants. This is the load-bearing capture for task
# 1-9: it puts each 1-9-pinned light surface tint (bg.selection #D0C6F0,
# bg.warning #E8D6A8, bg.track #D2D4DE, borders #C9CDDB) against #e1e2e7 — the
# light-tint-on-light-canvas wash-out risk a numeric pass alone cannot catch
# (§2.9 / §15.6). The §4.1 on-tint foregrounds (text.on-selection / text.strong /
# state.green-on-selection — the §2.8 darker on-selection override, the 1-9
# wash-out remedy — on bg.selection; text.on-warning on bg.warning) are rendered
# ON their tints for the eyeball. Like the dark tape this NEVER opens a tmux
# server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/contrast-validation-light.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/contrast-validation-light.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the swatch fixture in light mode.
# `go run` compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "go run ./cmd/capturetool --fixture contrast-validation --appearance light"
Enter

# First paint: the swatch renders the four tint bands on the light canvas. Give
# the compile + first render time to settle.
Sleep 4s

Screenshot "testdata/vhs/contrast-validation-light.png"
