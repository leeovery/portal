# vhs tape: Edit Project modal — editing a TAGS chip in place (§8.2 / §13.1).
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `projects` fixture (the fixture wires the in-memory ProjectEditor + AliasEditor so
# the edit modal can open). Switches to the Projects page (x), opens the edit-project
# modal (e) on `flow-v1-api`, navigates to the `Fabric` TAGS chip, then enters EDIT
# mode on it (e). The harness binds every seam to an in-memory fake, so this NEVER
# opens a tmux server or touches ~/.config/portal.
#
# Navigation per the 3-8 state machine:
#   - the modal opens with NAME focused;
#   - Tab -> ALIASES, Tab -> TAGS (Tab into a chip field lands on the `+ add` slot);
#   - the TAGS chips are [Fabric, api], so the `+ add` slot is at index 2 — two Left
#     presses walk back to the `Fabric` chip (index 0);
#   - e enters EDIT mode on the focused `Fabric` chip (Enter would do the same).
#
# The capture verifies the §13.1 editing-in-place state: the `Fabric` chip renders as
# a SQUARE bordered box in accent.orange with a live accent.orange block cursor (NO
# fill, NO ✕), the `◉ EDIT MODE` header badge appears in accent.orange (absent in
# navigate), and the footer is the editing variant
# `⏎ save · esc discard · ←→ cursor · empty on save = delete`. Compare against
# testdata/vhs/reference/edit-modal-edit-in-place-mv.png.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/edit-modal-edit-in-place.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/edit-modal-edit-in-place.gif"

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

# Switch Sessions -> Projects with the `x` key, then open the edit-project modal (e)
# on `flow-v1-api`. It opens with NAME focused.
Type "x"
Sleep 1s
Type "e"
Sleep 1s

# Tab to ALIASES, then to TAGS (each Tab into a chip field lands on the `+ add` slot).
Tab
Sleep 500ms
Tab
Sleep 500ms

# Two Left presses walk the TAGS cursor back to the `Fabric` chip (index 0).
Left
Sleep 500ms
Left
Sleep 500ms

# Enter EDIT mode on the focused `Fabric` chip (e — Enter is equivalent). The chip
# border turns accent.orange with a live block cursor, and the `◉ EDIT MODE` badge
# appears in the header.
Type "e"
Sleep 1s

Screenshot "testdata/vhs/edit-modal-edit-in-place.png"
