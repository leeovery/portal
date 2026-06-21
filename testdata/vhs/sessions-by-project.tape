# vhs tape: Sessions — by Project (grouped) view, DARK owned canvas.
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-by-project` fixture, which opens directly in By-Project mode
# (initialMode) with projects whose paths match most session dirs plus one session
# whose directory matches NO project (so the pinned Unknown catch-all heading
# renders). The capture shows the §3.2 section header's `— by project` mode suffix
# and the restyled group headings (heading in text.detail, `··· N` count in
# text.dim) with rows nested one indent level under their heading. Like the other
# fixtures this NEVER opens a tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/sessions-by-project.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/sessions-by-project.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the by-project fixture. The fixture opens
# in By-Project mode, so no key drive is needed to reach the grouped view — the
# first painted frame already shows the `— by project` section header + group
# headings + nested rows.
Type "go run ./cmd/capturetool --fixture sessions-by-project"
Enter

# First paint: the model fetches the fixture sessions + projects on Init and renders
# the grouped By-Project list. Give the compile + first render time to settle.
Sleep 4s

Screenshot "testdata/vhs/sessions-by-project.png"
