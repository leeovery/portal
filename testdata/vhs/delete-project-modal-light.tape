# vhs tape: Delete Project Confirm Modal — MV reskin, LIGHT owned canvas (§8.6 / §8.1 / §15.6).
#
# Mirror of delete-project-modal.tape but drives cmd/capturetool with
# --appearance light so the delete modal paints on the light mode-matched canvas
# (#e1e2e7). A human confirms the single-tone joined panel + the state.red header/
# name + accent.blue footer glyphs read correctly in light mode. Like the dark tape
# this NEVER opens a tmux server or touches ~/.config/portal.
#
# Compare structure against testdata/vhs/reference/kill-confirm-modal-light.png (no
# dedicated delete reference — it mirrors kill).
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/delete-project-modal-light.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/delete-project-modal-light.gif"

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
# Projects page render on the light canvas with the cursor on the first project.
Type "x"
Sleep 1s

# Open the delete-project confirm modal on the selected project. This clears the
# page behind it to the owned canvas and centres the MV panel on it (§8.1
# blank-screen layer).
Type "d"
Sleep 1s

Screenshot "testdata/vhs/delete-project-modal-light.png"
