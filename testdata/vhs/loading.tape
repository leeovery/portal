# vhs tape: Loading interstitial (§10.3 honest loading screen), DARK owned canvas.
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `loading-screen` fixture: the cold-boot loading page (serverStarted) seeded with
# the reference mid-restore progress — steps 1–2 done (✓ Started tmux server,
# ✓ Registered hooks), "Restoring sessions" active with the 8 / 12 counter, the
# trailing labels pending, the bar partially filled — mirroring
# testdata/vhs/reference/loading-mv.png (`Loading 6 — Combined (thick bar)`).
#
# The fixture wires a progress receiver that streams the seeded events then BLOCKS
# (never emits the terminal BootstrapCompleteMsg), so the dual-gate never dismisses
# the loading page and the screenshot stays parked on it. The capturetool pins the
# DARK owned canvas (--appearance default), so the page paints the correct canvas
# from frame one. The harness imports Portal's real internal/tui and binds every
# tmux seam to an in-memory fake, so this NEVER opens a tmux server, never spawns a
# daemon, and never touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/loading.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/loading.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the loading-screen fixture. `go run`
# compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "go run ./cmd/capturetool --fixture loading-screen"
Enter

# The seeded progress events stream on Init and fold into the loadingProgress
# accumulator; the receiver then blocks so the page stays on the loading screen.
# Give the compile + first render time to settle (well past the 1.2s min-display
# pad, which alone never dismisses — the terminal complete event never arrives).
Sleep 6s

Screenshot "testdata/vhs/loading.png"
