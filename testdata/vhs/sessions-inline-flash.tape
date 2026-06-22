# vhs tape: Sessions — inline flash band (§11.2 warning variant).
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-inline-flash` fixture and screenshots the first painted frame. The
# harness imports Portal's real internal/tui and binds every tmux seam to an
# in-memory fake, so this NEVER opens a tmux server or touches ~/.config/portal.
#
# This is the §11.2 inline-flash capture: the warning band (accent.orange ▌
# left-bar + ⚠ + "folio-Jiz4el closed externally — list updated" on a bg.warning
# tint, text.on-warning message) renders directly under the title separator, above
# the `Sessions 4` section header. The flash is otherwise transient (production
# sets it only on the preview-bail path), so the fixture seeds it via
# Deps.InitialFlash. Compare against
# testdata/vhs/reference/sessions-inline-flash-mv.png.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/sessions-inline-flash.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/sessions-inline-flash.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the deterministic fixture. `go run`
# compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "go run ./cmd/capturetool --fixture sessions-inline-flash"
Enter

# First paint: the model fetches the fixture sessions on Init and renders the flat
# list with the seeded warning flash band above the section header. Give the
# compile + first render time to settle.
Sleep 4s

Screenshot "testdata/vhs/sessions-inline-flash.png"
