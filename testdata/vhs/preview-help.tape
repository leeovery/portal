# vhs tape: Preview `?` help overlay (§8.5/§9.3) — the descriptor-driven help
# panel composited OVER the §9.1 cyan "peek mode" preview, DARK canvas.
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `preview-screen` fixture (same fixture preview-screen.tape uses): a Flat session
# list whose first (default-selected) session is aviva-proxy-qNyfEO, a single-
# window single-pane session seeded with generic canned scrollback. This tape
# presses Space to open the §9 preview overlay, then `?` to open the per-page help.
#
# The capture shows the help OVERLAYING the preview WITHOUT blanking it (§9 — the
# Preview `?` help overlays, it does NOT route through the §8.1 blank-screen path):
# the descriptor-driven generic help panel (the SAME renderHelpModalContent the
# Sessions/Projects help modals use — `? Keybindings` header, right-aligned
# `esc close`, the two-column glyph(accent.blue)/action(text.strong) keymap) sits
# centred ON TOP of the still-visible preview chrome — the ◉ preview marker, the
# cyan border + `├┤` dividers, and the captured scrollback remain visible behind
# the panel. The help lists the COMPLETE Preview keymap (§12.1) from the descriptor:
# `↑/↓ scroll`, `^↑/↓ page`, `Home/End top/bottom`, `←→ window`, `⇥ pane`,
# `⏎ attach`, `␣ back`.
#
# The harness imports Portal's real internal/tui and binds every tmux seam to an
# in-memory fake, so this NEVER opens a tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/preview-help.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/preview-help.gif"

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

# Press ? to open the per-page help OVERLAID on the preview (it must NOT blank the
# preview behind it), then let the composited panel render.
Type "?"
Sleep 2s

Screenshot "testdata/vhs/preview-help.png"
