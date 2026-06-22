# vhs tape: Edit Project modal — NAME focused, navigate mode (§8.2 / §13.1).
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `projects` fixture (a rich project store; the fixture opens on the Sessions page,
# and wires the in-memory ProjectEditor + AliasEditor so the edit modal can open).
# Switches to the Projects page (x), then opens the edit-project modal (e) on the
# first project (`flow-v1-api`), which lands in NAVIGATE mode with the NAME field
# focused. The harness imports Portal's real internal/tui and binds every seam to an
# in-memory fake, so this NEVER opens a tmux server or touches ~/.config/portal.
#
# The capture verifies the §13.1 MV grammar in the NAME-focused navigate state: the
# hand-drawn single-tone joined panel (Edit Project <name> header), the focused NAME
# label in accent.violet, the NAME input as a ROUNDED bordered box in accent.violet
# (no fill), the ALIASES/TAGS labels in text.detail, the chips as SQUARE bordered
# boxes in border.separator grey (text.primary, no ✕, never green), the faint `+ add`
# slot, and the name-focused footer `⏎/e edit · ⇥ next field · esc close` (key glyphs
# accent.blue, labels text.detail). Compare against
# testdata/vhs/reference/edit-modal-navigate-name-mv.png.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/edit-modal-navigate-name.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/edit-modal-navigate-name.gif"

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

# Open the edit-project modal on the selected project. It opens in NAVIGATE mode with
# the NAME field focused (the default landing state) — no further keys needed.
Type "e"
Sleep 1s

Screenshot "testdata/vhs/edit-modal-navigate-name.png"
