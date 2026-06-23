# vhs tape: Sessions — §11.1 empty-sessions state (MV reskin).
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-empty` fixture (ZERO sessions) and screenshots the first painted
# frame: the centred `▌ ▌ ▌` block glyph (text.faint) + `No sessions yet`
# (text.primary) + the hint (text.detail), with the FULLY-REPLACED footer
# (`n new in cwd · x projects · / filter · ? help`). The harness imports
# Portal's real internal/tui and binds every tmux seam to an in-memory fake, so
# this NEVER opens a tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/sessions-empty.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/sessions-empty.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the deterministic empty fixture. `go
# run` compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "go run ./cmd/capturetool --fixture sessions-empty"
Enter

# First paint: with ZERO sessions the model auto-routes to the Projects page on
# launch (model.go default-page rule: no sessions -> PageProjects). Give the
# compile + first render time to settle, then press `x` to navigate to the
# Sessions page, where the §11.1 empty-SESSIONS state renders (the reference
# surface). The default-page rule is a one-time init flag, so `x` stays on
# Sessions and does not bounce back.
Sleep 4s
Type "x"
Sleep 1s

Screenshot "testdata/vhs/sessions-empty.png"
