# vhs tape: Rename Modal — MV reskin (§8.4 / §8.1 / §13.1).
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-flat` fixture, navigates the cursor to `aviva-proxy-qNyfEO` (the
# reference target), opens the rename modal (r), clears the pre-filled name and
# types `aviva-proxy` (matching the reference value), then screenshots the result.
# The harness imports Portal's real internal/tui and binds every tmux seam to an
# in-memory fake (the fixture wires a Renamer), so this NEVER opens a tmux server
# or touches ~/.config/portal.
#
# The capture verifies the §8.4 reskin: the hand-drawn single-tone joined panel
# (Rename session header + right-aligned `◉ EDIT MODE` badge / NEW NAME label +
# orange-outlined input box with an orange block cursor (the shared renderInputBox
# always-editing state) + was: <old name> body / ⏎ rename · esc cancel footer), all
# in border.separator with joined ├───┤ dividers, centred on the cleared owned
# canvas. Compare against testdata/vhs/reference/rename-modal-mv.png.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/rename-modal.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/rename-modal.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the deterministic fixture. `go run`
# compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "go run ./cmd/capturetool --fixture sessions-flat"
Enter

# First paint: the model fetches the fixture sessions on Init and renders the flat
# list. Give the compile + first render time to settle before opening the modal.
Sleep 4s

# Move the cursor to `aviva-proxy-qNyfEO` (the 5th flat row, index 4) — the §8.4
# reference target.
Down
Down
Down
Down
Sleep 500ms

# Open the rename modal on the selected session. The input pre-fills with the old
# name; this clears the page behind it to the owned canvas and centres the MV panel
# on it (§8.1 blank-screen layer).
Type "r"
Sleep 500ms

# Clear the pre-filled name and type the reference value `aviva-proxy` so the input
# box + orange block cursor render exactly like the Paper frame.
Ctrl+U
Type "aviva-proxy"
Sleep 1s

Screenshot "testdata/vhs/rename-modal.png"
