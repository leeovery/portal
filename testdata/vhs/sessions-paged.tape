# vhs tape: Sessions — Multi-page list (centred pagination dots).
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-paged` fixture (40 sessions, spanning >1 page at this size) and
# screenshots the first painted frame. The harness imports Portal's real
# internal/tui and binds every tmux seam to an in-memory fake, so this NEVER opens
# a tmux server or touches ~/.config/portal.
#
# This is the §3.5 pagination-dots capture: the multi-page list renders the
# centred dot row above the condensed footer — active dot accent.violet, inactive
# dots text.faint, no full-screen frame. Compare against
# Sessions — Modern Vivid v2 / (Light) (the pagination region).
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/sessions-paged.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/sessions-paged.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the deterministic fixture. `go run`
# compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "go run ./cmd/capturetool --fixture sessions-paged"
Enter

# First paint: the model fetches the fixture sessions on Init and renders the
# flat list. The 40-session set spans >1 page, so the centred dot row renders
# above the footer. Give the compile + first render time to settle.
Sleep 4s

Screenshot "testdata/vhs/sessions-paged.png"
