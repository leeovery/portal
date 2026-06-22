# vhs tape: Edit Project modal — a TAGS chip focused, navigate mode (§8.2 / §13.1).
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `projects` fixture (the fixture wires the in-memory ProjectEditor + AliasEditor so
# the edit modal can open). Switches to the Projects page (x), opens the edit-project
# modal (e) on `flow-v1-api`, then navigates to the `Fabric` TAGS chip and focuses it.
# The harness binds every seam to an in-memory fake, so this NEVER opens a tmux server
# or touches ~/.config/portal.
#
# Navigation per the 3-8 navigate-mode state machine:
#   - the modal opens with NAME focused;
#   - Tab -> ALIASES, Tab -> TAGS (Tab into a chip field lands on the trailing `+ add`
#     slot, NOT the first chip);
#   - the TAGS chips are [Fabric, api] (deterministic order), so the `+ add` slot is
#     at element index 2 — two Left presses walk back: add-slot (2) -> api (1) ->
#     Fabric (0), landing focus on the `Fabric` chip.
#
# The capture verifies the §13.1 focused-chip state: the `Fabric` chip renders as a
# SQUARE bordered box in accent.violet (purple) — NO fill, NO inline ✕ — the other
# chips stay border.separator grey, the focused TAGS label is accent.violet, and the
# footer is the chip-focused variant
# `⏎/e edit · x remove · ←→ move · ⇥ next field · esc close`. Compare against
# testdata/vhs/reference/edit-modal-chip-focused-mv.png.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/edit-modal-chip-focused.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/edit-modal-chip-focused.gif"

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

# Two Left presses walk the TAGS cursor back: add-slot (2) -> api (1) -> Fabric (0).
Left
Sleep 500ms
Left
Sleep 1s

Screenshot "testdata/vhs/edit-modal-chip-focused.png"
