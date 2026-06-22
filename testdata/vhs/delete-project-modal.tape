# vhs tape: Delete Project Confirm Modal — MV reskin (§8.6 / §8.1).
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `projects` fixture (a rich project store; the fixture opens on the Sessions page),
# switches to the Projects page (x), then opens the delete-project confirm modal (d)
# on the first project (`flow-v1-api`), and screenshots the result. The harness
# imports Portal's real internal/tui and binds every seam to an in-memory fake (the
# fixture wires a fakeProjectStore whose Remove is a no-op), so this NEVER opens a
# tmux server or touches ~/.config/portal.
#
# The capture verifies the §8.6 reskin, MIRRORING the kill modal: the hand-drawn
# single-tone joined panel (▲ Delete project? header in state.red / <name> in
# state.red + <path> in text.detail + the RECORD-ONLY consequence body / y delete ·
# esc cancel footer), all in border.separator with joined ├───┤ dividers, centred on
# the cleared owned canvas, no fill. Compare structure against
# testdata/vhs/reference/kill-confirm-modal-mv.png (no dedicated delete reference —
# it mirrors kill).
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/delete-project-modal.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/delete-project-modal.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the projects fixture. The fixture opens on
# the Sessions page; give the compile + first render time to settle.
Type "go run ./cmd/capturetool --fixture projects"
Enter
Sleep 4s

# Switch Sessions -> Projects with the `x` key (the real user path), then let the
# Projects page render with the cursor on the first project (`flow-v1-api`).
Type "x"
Sleep 1s

# Open the delete-project confirm modal on the selected project. This clears the
# page behind it to the owned canvas and centres the MV panel on it (§8.1
# blank-screen layer).
Type "d"
Sleep 1s

Screenshot "testdata/vhs/delete-project-modal.png"
