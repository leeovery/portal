# vhs tape: Edit Project modal — NAME focused, navigate mode, LIGHT owned canvas
# (§8.2 / §13.1 / §15.6).
#
# Mirror of edit-modal-navigate-name.tape but drives cmd/capturetool with
# --appearance light so the edit modal paints on the light mode-matched canvas
# (#e1e2e7). A human confirms the §13.1 MV grammar reads correctly in light mode:
# the single-tone joined panel (Edit Project <name> header), the focused NAME label
# in accent.violet, the NAME input as a ROUNDED accent.violet bordered box (no fill),
# the ALIASES/TAGS labels in text.detail, the chips as SQUARE border.separator grey
# boxes (text.primary, no ✕, never green), the faint `+ add` slot, and the
# name-focused footer `⏎/e edit · ⇥ next field · esc close`. There is NO light Paper
# reference for the edit modal, so this is the §15.6 light-mode legibility/structure
# eyeball (per-token coverage), not a frame compare. Like the dark tape this NEVER
# opens a tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/edit-modal-navigate-name-light.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/edit-modal-navigate-name-light.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the projects fixture in light mode. The
# fixture opens on the Sessions page; give the compile + first render time to settle.
Type "go run ./cmd/capturetool --fixture projects --appearance light"
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

Screenshot "testdata/vhs/edit-modal-navigate-name-light.png"
