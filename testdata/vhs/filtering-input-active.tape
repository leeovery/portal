# vhs tape: Filtering — input-active (MV), DARK owned canvas.
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-flat` fixture (it already carries the fab* sessions the §7.1 reference
# shows), presses `/` to open the inline filter input, then types `fab`. The
# captured frame is the §7 / §7.1 input-active state: the accent.orange `/` prompt
# + the `fab` query in accent.orange with the cursor at end-of-text, NO list row
# selected, and the input-active footer
# (`type to filter · ↵ / ↓ browse results · esc clear`). Like the other fixtures
# this NEVER opens a tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/filtering-input-active.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/filtering-input-active.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the deterministic flat fixture. `go run`
# compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "go run ./cmd/capturetool --fixture sessions-flat"
Enter

# First paint: the model fetches the fixture sessions on Init and renders the flat
# list. Give the compile + first render time to settle before driving the filter.
Sleep 4s

# Open the inline filter input (input-active) and type the partial query.
Type "/"
Sleep 500ms
Type "fab"
Sleep 1s

Screenshot "testdata/vhs/filtering-input-active.png"
