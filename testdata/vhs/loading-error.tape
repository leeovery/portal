# vhs tape: Fatal cold-boot error frame (§10.5), DARK owned canvas.
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `loading-error` fixture: the cold-boot loading page (serverStarted) advanced to
# steps 1–2 done (✓ Started tmux server, ✓ Registered hooks), then a FATAL abort at
# step 3 (set @portal-restoring) — so the "Registered hooks" row carries the
# state.red ✗ marker, the trailing labels stay pending (·, never ran), the
# one-line FatalError.UserMessage renders beneath the step-list in state.red, and a
# `q quit · esc quit` hint sits at the bottom. The bar freezes at the fatal-time
# fraction (2/11). This frame is MOCKED at implementation — there is NO §15.1 Paper
# reference for it; the capture IS the mock.
#
# The fixture wires a progress receiver that streams the two progress events then
# the terminal BootstrapFatalMsg, then BLOCKS — so the model enters the error state
# (no terminal BootstrapCompleteMsg, no transition to the picker) and the
# screenshot stays parked on the error frame. The capturetool pins the DARK owned
# canvas (--appearance default), so the page paints the correct canvas from frame
# one. The harness imports Portal's real internal/tui and binds every tmux seam to
# an in-memory fake, so this NEVER opens a tmux server, never spawns a daemon, and
# never touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/loading-error.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/loading-error.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the loading-error fixture. `go run`
# compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "go run ./cmd/capturetool --fixture loading-error"
Enter

# The seeded progress events stream on Init and fold into the loadingProgress
# accumulator, then the terminal fatal enters the §10.5 error state; the receiver
# then blocks so the page stays parked on the error frame. Give the compile +
# first render time to settle (well past the 1.2s min-display pad, which alone
# never dismisses — and a fatal never dismisses at all).
Sleep 6s

Screenshot "testdata/vhs/loading-error.png"
