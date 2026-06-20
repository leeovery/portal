# vhs tape: Sessions — by Tag (grouped) view, DARK owned canvas.
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-by-tag` fixture, which opens directly in By-Tag mode (initialMode) with
# tagged projects whose paths match the session dirs. The capture shows the §3.2
# section header's `— by tag` mode suffix and the restyled group headings, so the
# mode-suffix colour role is verifiable. Like the flat tapes this NEVER opens a
# tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/sessions-by-tag.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/sessions-by-tag.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the by-tag fixture. The fixture opens in
# By-Tag mode, so no key drive is needed to reach the grouped view — the first
# painted frame already shows the `— by tag` section header + group headings.
Type "go run ./cmd/capturetool --fixture sessions-by-tag"
Enter

# First paint: the model fetches the fixture sessions + tagged projects on Init and
# renders the grouped By-Tag list. Give the compile + first render time to settle.
Sleep 4s

Screenshot "testdata/vhs/sessions-by-tag.png"
