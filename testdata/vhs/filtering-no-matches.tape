# vhs tape: Filtering — no matches (MV), DARK owned canvas.
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-flat` fixture (the fab*/agentic*/evvi* session set), presses `/` to open
# the inline filter input, then types a query that matches NOTHING (`zzqx`). The
# captured frame is the §7.3 over-filtered no-matches state: the input-active
# accent.orange `/ zzqx` prompt with the cursor at end-of-text, then — centred in the
# body — the dim null-set glyph `∅` (text.faint) over `No sessions match "zzqx"`
# (text.primary, bold) over `⌫ to widen the search · esc to clear the filter`
# (text.detail), with the footer staying in the REDUCED input-active form
# (`type to filter · esc clear` — no `browse results`, there are none to browse).
# Like the other fixtures this NEVER opens a tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/filtering-no-matches.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/filtering-no-matches.gif"

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

# Open the inline filter input (input-active) and type a query that matches nothing.
Type "/"
Sleep 500ms
Type "zzqx"
Sleep 1s

Screenshot "testdata/vhs/filtering-no-matches.png"
