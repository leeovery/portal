# vhs tape: Sessions — "No tags yet" signpost (§11.3), DARK owned canvas.
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-no-tags-signpost` fixture, which opens directly in By-Tag mode
# (initialMode) with projects that carry NO tags. Because no project anywhere
# carries a tag, the By-Tag view degrades to the flat session list under the
# persistent accent.violet "No tags yet" info signpost (degrade with message, not
# silent flatten — §5.3), funnelled through the §11 single-slot notice-band
# arbiter. The capture shows the violet ▌ left-bar band + message (text.strong, no
# tint) under the title separator, above the `Sessions — by tag` section header,
# over the flat list. Like the other tapes this NEVER opens a tmux server or
# touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/sessions-no-tags-signpost.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/sessions-no-tags-signpost.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the no-tags-signpost fixture. The fixture
# opens in By-Tag mode with zero tags, so no key drive is needed to reach the
# signposted view — the first painted frame already shows the violet info band over
# the flat list.
Type "go run ./cmd/capturetool --fixture sessions-no-tags-signpost"
Enter

# First paint: the model fetches the fixture sessions + untagged projects on Init
# and renders the flat list under the "No tags yet" signpost. Give the compile +
# first render time to settle.
Sleep 4s

Screenshot "testdata/vhs/sessions-no-tags-signpost.png"
