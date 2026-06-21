# vhs tape: Filtering — list-active (MV), DARK owned canvas.
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-flat` fixture (it already carries the fab* sessions the §7.1 reference
# shows), presses `/` to open the inline filter input, types `fab`, then presses
# Enter to COMMIT the filter (Filtering → FilterApplied). The captured frame is the
# §7 / §7.1 list-active state: the LOCKED accent.orange `/ fab` query with NO
# cursor, a row IS selected (the §3.3 violet ▌ bar + bg.selection band), the filter
# input itself carries NO background tint, and the list-active footer
# (`↵ attach · ↑↓ navigate · esc clear filter`). Like the other fixtures this NEVER
# opens a tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/filtering-list-active.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/filtering-list-active.gif"

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

# Open the inline filter input, type the partial query, then Enter to commit the
# filter — input-active → list-active (locked query, selectable rows).
Type "/"
Sleep 500ms
Type "fab"
Sleep 500ms
Enter
Sleep 1s

Screenshot "testdata/vhs/filtering-list-active.png"
