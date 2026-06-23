# vhs tape: Preview screen (MV reskin) — §9.1 cyan "peek mode" overlay, DARK canvas.
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `preview-screen` fixture: a Flat session list whose first (default-selected)
# session is aviva-proxy-qNyfEO, a single-window single-pane session seeded with
# generic canned scrollback. This tape presses Space on that session to open the
# §9 preview overlay, then screenshots it. The capture shows the §9.1 full-screen
# single-tone accent.cyan joined panel: a HEADER compartment (◉ preview marker +
# session name + `Window 1/1 · Pane 1/1` counters), the BODY (the UNTOUCHED
# captured ANSI scrollback, §9.2), and a FOOTER compartment of nav hints
# (accent.blue glyphs + text.detail labels, space-separated:
# `←→ window  ⌃←→ pane  ⏎ attach  ␣ back`) — all wrapped in the cyan border +
# joined `├┤` dividers. The window/pane bindings are now ←/→ (window) and
# Ctrl+←/Ctrl+→ (pane); Space (open) is unchanged, so this tape needs no
# key-sequence change. The harness imports Portal's real internal/tui and binds
# every tmux seam to an in-memory fake, so this NEVER opens a tmux server or
# touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/preview-screen.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/preview-screen.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the preview-screen fixture. The fixture
# opens on the Sessions page with aviva-proxy-qNyfEO selected; give the compile +
# first render time to settle.
Type "go run ./cmd/capturetool --fixture preview-screen"
Enter
Sleep 4s

# Press Space on the selected session to open the §9 preview overlay, then let the
# cyan peek-mode chrome + captured scrollback render.
Space
Sleep 2s

Screenshot "testdata/vhs/preview-screen.png"
